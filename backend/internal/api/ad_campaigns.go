package api

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"named_clocks/backend/internal/adcampaign"
	"named_clocks/backend/internal/store"
	"named_clocks/backend/internal/vkads"
)

type createAdCampaignRequest struct {
	Title               string            `json:"title"`
	Names               []string          `json:"names"`
	Surnames            []string          `json:"surnames"`
	NameTextTemplate    string            `json:"nameTextTemplate"`
	SurnameTextTemplate string            `json:"surnameTextTemplate"`
	TemplateID          string            `json:"templateId"`
	ImageSettings       map[string]string `json:"imageSettings"`
	NameSettingKey      string            `json:"nameSettingKey"`
	VideoModel          string            `json:"videoModel"`
	VideoPrompt         string            `json:"videoPrompt"`
	VideoDuration       *int              `json:"videoDuration"`
	VideoResolution     string            `json:"videoResolution"`
	VideoAspectRatio    string            `json:"videoAspectRatio"`
	GenerateAudio       bool              `json:"generateAudio"`
	AudioAssetID        string            `json:"audioAssetId"`
	VKSettings          vkads.Settings    `json:"vkSettings"`
}

func (s *Server) handleAdCampaignDefaults(w http.ResponseWriter, _ *http.Request) {
	names, surnames, nameDiagnostics, surnameDiagnostics := adcampaign.Defaults()
	writeJSON(w, http.StatusOK, map[string]any{
		"names":               names,
		"surnames":            surnames,
		"nameTextTemplate":    adcampaign.DefaultNameText,
		"surnameTextTemplate": adcampaign.DefaultSurnameText,
		"diagnostics": map[string]any{
			"names":    nameDiagnostics,
			"surnames": surnameDiagnostics,
		},
		"vkSettings": vkads.DefaultSettings(),
	})
}

func (s *Server) handleCreateAdCampaign(w http.ResponseWriter, r *http.Request) {
	var req createAdCampaignRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.TemplateID = strings.TrimSpace(req.TemplateID)
	req.NameTextTemplate = strings.TrimSpace(req.NameTextTemplate)
	req.SurnameTextTemplate = strings.TrimSpace(req.SurnameTextTemplate)
	names, nameDiagnostics := adcampaign.Normalize(req.Names)
	surnames, surnameDiagnostics := adcampaign.Normalize(req.Surnames)
	switch {
	case req.Title == "":
		writeError(w, http.StatusBadRequest, "title is required")
		return
	case len(names) == 0 && len(surnames) == 0:
		writeError(w, http.StatusBadRequest, "at least one names or surnames list is required")
		return
	case req.TemplateID == "":
		writeError(w, http.StatusBadRequest, "templateId is required")
		return
	case len(names) > 0 && !adcampaign.HasNamePlaceholder(req.NameTextTemplate):
		writeError(w, http.StatusBadRequest, "nameTextTemplate must contain {{name}}")
		return
	case len(surnames) > 0 && !adcampaign.HasNamePlaceholder(req.SurnameTextTemplate):
		writeError(w, http.StatusBadRequest, "surnameTextTemplate must contain {{name}}")
		return
	}
	if req.VideoModel == "" {
		req.VideoModel = s.defaultModel
	}
	if strings.TrimSpace(req.VideoPrompt) == "" {
		req.VideoPrompt = s.defaultPrompt
	}
	if req.VideoDuration == nil && s.defaultDuration > 0 {
		d := s.defaultDuration
		req.VideoDuration = &d
	}
	req.NameSettingKey = orDefault(req.NameSettingKey, "name")
	if req.ImageSettings == nil {
		req.ImageSettings = map[string]string{}
	}
	audioObject := ""
	if req.GenerateAudio {
		req.AudioAssetID = ""
	} else {
		req.AudioAssetID = strings.TrimSpace(req.AudioAssetID)
		if req.AudioAssetID == "" {
			writeError(w, http.StatusBadRequest, "выберите mp3 для озвучки или включите генерацию аудио моделью")
			return
		}
		asset, err := s.store.GetMediaAsset(r.Context(), req.AudioAssetID)
		if err != nil {
			log.Printf("api: get campaign media asset %s: %v", req.AudioAssetID, err)
			writeError(w, http.StatusInternalServerError, "could not load audio")
			return
		}
		if asset == nil {
			writeError(w, http.StatusBadRequest, "выбранный mp3 не найден")
			return
		}
		audioObject = asset.Object
	}
	campaign := &store.AdCampaign{
		Title: req.Title, NameTextTemplate: req.NameTextTemplate, SurnameTextTemplate: req.SurnameTextTemplate,
		TemplateID: req.TemplateID, ImageSettings: req.ImageSettings, NameSettingKey: req.NameSettingKey,
		VideoModel: req.VideoModel, VideoPrompt: req.VideoPrompt,
		VideoDuration: req.VideoDuration, VideoResolution: req.VideoResolution, VideoAspectRatio: req.VideoAspectRatio,
		GenerateAudio: req.GenerateAudio, AudioAssetID: req.AudioAssetID, AudioObject: audioObject,
		VKSettings: req.VKSettings.Normalize(),
	}
	if err := s.store.CreateAdCampaign(r.Context(), campaign, names, surnames); err != nil {
		log.Printf("api: create ad campaign: %v", err)
		writeError(w, http.StatusInternalServerError, "could not create ad campaign")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"campaign":    campaign,
		"diagnostics": map[string]any{"names": nameDiagnostics, "surnames": surnameDiagnostics},
	})
}

func (s *Server) handleListAdCampaigns(w http.ResponseWriter, r *http.Request) {
	campaigns, err := s.store.ListAdCampaigns(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list ad campaigns")
		return
	}
	rate := s.rater.Rate(r.Context())
	for _, campaign := range campaigns {
		campaign.CostRUB = campaign.CostUSD * rate
	}
	writeJSON(w, http.StatusOK, map[string]any{"campaigns": campaigns, "usdRubRate": rate})
}

func (s *Server) handleGetAdCampaign(w http.ResponseWriter, r *http.Request) {
	campaign, err := s.store.GetAdCampaign(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load ad campaign")
		return
	}
	if campaign == nil {
		writeError(w, http.StatusNotFound, "ad campaign not found")
		return
	}
	rate := s.rater.Rate(r.Context())
	campaign.CostRUB = campaign.CostUSD * rate
	writeJSON(w, http.StatusOK, map[string]any{"campaign": campaign, "usdRubRate": rate})
}

func (s *Server) handleListAdCampaignItems(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	kind, status := r.URL.Query().Get("kind"), r.URL.Query().Get("status")
	if kind != "" && kind != "name" && kind != "surname" {
		writeError(w, http.StatusBadRequest, "kind must be name or surname")
		return
	}
	items, total, err := s.store.ListAdCampaignItems(r.Context(), chi.URLParam(r, "id"), kind, status, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list ad campaign items")
		return
	}
	rate := s.rater.Rate(r.Context())
	for _, item := range items {
		item.CostRUB = item.CostUSD * rate
		base := sanitizeFileName(item.Kind + "_" + item.Value)
		if base == "" {
			base = item.Kind
		}
		item.ImageDownloadURL = s.presignCampaignObject(r, item.ImageObject, base+filepath.Ext(item.ImageObject))
		item.SourceDownloadURL = s.presignCampaignObject(r, item.SourceVideoObject, base+"_source.mp4")
		item.VideoDownloadURL = s.presignCampaignObject(r, item.VideoObject, base+".mp4")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "limit": normalizedCampaignLimit(limit), "offset": max(offset, 0), "usdRubRate": rate,
	})
}

func (s *Server) presignCampaignObject(r *http.Request, object, filename string) string {
	if object == "" {
		return ""
	}
	url, err := s.storage.PresignedDownloadURL(r.Context(), object, filename, 24*time.Hour)
	if err != nil {
		log.Printf("api: presign campaign object %s: %v", object, err)
		return ""
	}
	return url
}

func normalizedCampaignLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func (s *Server) handleStartAdCampaign(w http.ResponseWriter, r *http.Request) {
	if s.vkads == nil {
		writeError(w, http.StatusConflict, "VK Ads не настроен: задайте ZALEY_SECRET и ZALEY_ACCOUNT_NAME")
		return
	}
	n, err := s.store.StartAdCampaign(r.Context(), chi.URLParam(r, "id"))
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "ad campaign not found")
		return
	}
	if errors.Is(err, store.ErrCampaignNotDraft) {
		writeError(w, http.StatusConflict, "ad campaign is not in draft state")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start ad campaign")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "running", "queued": n})
}

func (s *Server) handleIgnoreAdCampaignFailures(w http.ResponseWriter, r *http.Request) {
	err := s.store.IgnoreAdCampaignFailures(r.Context(), chi.URLParam(r, "id"))
	switch {
	case err == sql.ErrNoRows:
		writeError(w, http.StatusNotFound, "ad campaign not found")
	case errors.Is(err, store.ErrCampaignNotRunning):
		writeError(w, http.StatusConflict, "кампания ещё не запущена или уже завершена")
	case errors.Is(err, store.ErrCampaignInProgress):
		writeError(w, http.StatusConflict, "дождитесь окончания всех задач: они должны быть либо готовы, либо в ошибке")
	case errors.Is(err, store.ErrNothingToUpload):
		writeError(w, http.StatusConflict, "нет успешных задач для загрузки в ВКР")
	case err != nil:
		log.Printf("api: ignore campaign failures: %v", err)
		writeError(w, http.StatusInternalServerError, "could not ignore campaign failures")
	default:
		writeJSON(w, http.StatusOK, map[string]any{"status": "ignored"})
	}
}

func (s *Server) handleRetryAdCampaign(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.RetryAdCampaign(r.Context(), chi.URLParam(r, "id"))
	switch {
	case err == sql.ErrNoRows:
		writeError(w, http.StatusNotFound, "ad campaign not found")
	case errors.Is(err, store.ErrCampaignUploadStarted):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not retry ad campaign")
	default:
		writeJSON(w, http.StatusOK, map[string]any{"retried": n})
	}
}

func (s *Server) handleRetryAdCampaignItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	status, err := s.store.RetryAdCampaignItem(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrCampaignUploadStarted):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not retry ad campaign item")
	case status == "":
		writeError(w, http.StatusConflict, "item not found or not in failed state")
	default:
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": status})
	}
}

func (s *Server) handleDeleteAdCampaign(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.storage.RemovePrefix(r.Context(), "campaigns/"+id+"/"); err != nil {
		log.Printf("api: remove campaign objects %s: %v", id, err)
	}
	deleted, err := s.store.DeleteAdCampaign(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete ad campaign")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "ad campaign not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}
