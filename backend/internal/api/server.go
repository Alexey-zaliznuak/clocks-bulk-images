package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"named_clocks/backend/internal/auth"
	"named_clocks/backend/internal/currency"
	"named_clocks/backend/internal/media"
	"named_clocks/backend/internal/openrouter"
	"named_clocks/backend/internal/storage"
	"named_clocks/backend/internal/store"
	"named_clocks/backend/internal/vkads"
)

type Server struct {
	store           *store.Store
	auth            *auth.Authenticator
	openrouter      *openrouter.Client
	storage         *storage.Storage
	ffmpeg          *media.FFmpeg
	rater           *currency.Rater
	vkads           *vkads.Service
	defaultModel    string
	defaultDuration int
	defaultPrompt   string
	maxAudioMB      int64
	maxVideoMB      int64
}

// Options carries the server's tunables.
type Options struct {
	DefaultModel    string
	DefaultDuration int
	MaxAudioMB      int64
	MaxVideoMB      int64
	VKAds           *vkads.Service
}

// DefaultVideoPrompt is the out-of-the-box prompt for animating the clock image.
// The soundtrack is added later from the media library, so the model is asked for
// silent footage only.
const DefaultVideoPrompt = "оживи картинку, рука должна плавно и естественно двигаться, показывая часы с разных сторон. Надписи на циферблате строго без искажений и изменений. Секундная стрелка двигается медленно реалистично, строго по часовой стороне."

func NewServer(
	st *store.Store,
	a *auth.Authenticator,
	or *openrouter.Client,
	strg *storage.Storage,
	ff *media.FFmpeg,
	rater *currency.Rater,
	o Options,
) *Server {
	if o.MaxAudioMB <= 0 {
		o.MaxAudioMB = 50
	}
	if o.MaxVideoMB <= 0 {
		o.MaxVideoMB = 500
	}
	return &Server{
		store:           st,
		auth:            a,
		openrouter:      or,
		storage:         strg,
		ffmpeg:          ff,
		rater:           rater,
		vkads:           o.VKAds,
		defaultModel:    o.DefaultModel,
		defaultDuration: o.DefaultDuration,
		defaultPrompt:   DefaultVideoPrompt,
		maxAudioMB:      o.MaxAudioMB,
		maxVideoMB:      o.MaxVideoMB,
	}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/api/health", s.handleHealth)
	r.Post("/api/auth/login", s.handleLogin)

	r.Group(func(pr chi.Router) {
		pr.Use(s.auth.Middleware)
		pr.Get("/api/config", s.handleConfig)
		pr.Get("/api/vk-ads/status", s.handleVKAdsStatus)
		pr.Get("/api/vk-ads/pads", s.handleVKAdsPads)
		pr.Get("/api/vk-ads/cabinet", s.handleVKAdsCabinet)
		pr.Get("/api/models", s.handleModels)
		pr.Post("/api/tasks/batch", s.handleCreateBatch)
		pr.Get("/api/tasks", s.handleListTasks)
		pr.Post("/api/tasks/{id}/retry", s.handleRetryTask)
		pr.Get("/api/batches", s.handleListBatches)
		pr.Get("/api/batches/{id}", s.handleGetBatch)
		pr.Post("/api/batches/{id}/retry", s.handleRetryBatch)
		pr.Post("/api/batches/{id}/archive", s.handleBatchArchive)
		pr.Delete("/api/batches/{id}", s.handleDeleteBatch)

		pr.Get("/api/ad-campaigns/defaults", s.handleAdCampaignDefaults)
		pr.Post("/api/ad-campaigns", s.handleCreateAdCampaign)
		pr.Get("/api/ad-campaigns", s.handleListAdCampaigns)
		pr.Get("/api/ad-campaigns/{id}", s.handleGetAdCampaign)
		pr.Get("/api/ad-campaigns/{id}/items", s.handleListAdCampaignItems)
		pr.Post("/api/ad-campaigns/{id}/start", s.handleStartAdCampaign)
		pr.Post("/api/ad-campaigns/{id}/ignore-failed", s.handleIgnoreAdCampaignFailures)
		pr.Post("/api/ad-campaigns/{id}/retry", s.handleRetryAdCampaign)
		pr.Post("/api/ad-campaigns/items/{id}/retry", s.handleRetryAdCampaignItem)
		pr.Delete("/api/ad-campaigns/{id}", s.handleDeleteAdCampaign)

		pr.Get("/api/media/audio", s.handleListAudio)
		pr.Post("/api/media/audio", s.handleUploadAudio)
		pr.Delete("/api/media/audio/{id}", s.handleDeleteAudio)
		pr.Post("/api/media/extract-audio", s.handleExtractAudio)
	})

	return r
}

// ---------- handlers ----------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	token, err := s.auth.Login(req.Login, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	vkAds := map[string]any{"configured": false}
	if s.vkads != nil {
		vkAds["configured"] = true
		vkAds["accountName"] = s.vkads.AccountName()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"defaultModel":    s.defaultModel,
		"defaultDuration": s.defaultDuration,
		"defaultPrompt":   s.defaultPrompt,
		"vkAds":           vkAds,
	})
}

func (s *Server) handleVKAdsStatus(w http.ResponseWriter, r *http.Request) {
	if s.vkads == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"configured": false,
			"ok":         false,
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	user, err := s.vkads.CurrentUser(ctx)
	if err != nil {
		log.Printf("api: vk ads status: %v", err)
		writeJSON(w, http.StatusOK, map[string]any{
			"configured":  true,
			"ok":          false,
			"accountName": s.vkads.AccountName(),
			"error":       err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":  true,
		"ok":          true,
		"accountName": s.vkads.AccountName(),
		"user":        user,
	})
}

// handleVKAdsCabinet hands the UI what it needs to link an uploaded campaign
// into the cabinet. The sudo switch is the reason this is a request and not a
// constant: it names the account behind the agency token.
func (s *Server) handleVKAdsCabinet(w http.ResponseWriter, r *http.Request) {
	if s.vkads == nil {
		writeJSON(w, http.StatusOK, map[string]any{"configured": false})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	cabinet, err := s.vkads.CabinetLinks(ctx)
	if err != nil {
		log.Printf("api: vk ads cabinet: %v", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": true,
		"baseUrl":    cabinet.BaseURL,
		"sudo":       cabinet.Sudo,
	})
}

func (s *Server) handleVKAdsPads(w http.ResponseWriter, r *http.Request) {
	if s.vkads == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"configured": false,
			"trees":      []vkads.PadNode{},
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	settings := vkads.Settings{TargetAction: r.URL.Query().Get("targetAction")}
	opts, err := s.vkads.PlacementTreeForSettings(ctx, settings)
	if err != nil {
		log.Printf("api: vk ads pads: %v", err)
		writeError(w, http.StatusBadGateway, "не удалось загрузить места размещения ВКР")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":  true,
		"trees":       opts.Trees,
		"packageName": opts.Package.Name,
		"defaultPads": opts.Default,
	})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	models, err := s.openrouter.ListModels(ctx)
	if err != nil {
		log.Printf("api: list models: %v", err)
		writeError(w, http.StatusBadGateway, "could not fetch models from OpenRouter")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"models":       models,
		"defaultModel": s.defaultModel,
	})
}

type nameInput struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

type createBatchRequest struct {
	Title            string `json:"title"`
	TemplateID       string `json:"templateId"`
	VideoModel       string `json:"videoModel"`
	VideoPrompt      string `json:"videoPrompt"`
	VideoDuration    *int   `json:"videoDuration"`
	VideoResolution  string `json:"videoResolution"`
	VideoAspectRatio string `json:"videoAspectRatio"`
	// GenerateAudio asks the model for a soundtrack instead of muxing one from
	// the media library. Off by default because it costs noticeably more.
	GenerateAudio bool              `json:"generateAudio"`
	AudioAssetID  string            `json:"audioAssetId"`
	ExtraSettings map[string]string `json:"extraSettings"`
	FirstNameKey  string            `json:"firstNameKey"`
	LastNameKey   string            `json:"lastNameKey"`
	FullNameKey   string            `json:"fullNameKey"`
	Names         []nameInput       `json:"names"`
}

func (s *Server) handleCreateBatch(w http.ResponseWriter, r *http.Request) {
	var req createBatchRequest
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.TemplateID = strings.TrimSpace(req.TemplateID)
	if req.TemplateID == "" {
		writeError(w, http.StatusBadRequest, "templateId is required")
		return
	}
	if len(req.Names) == 0 {
		writeError(w, http.StatusBadRequest, "names list is empty")
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

	// Without model audio the clip comes back silent, so a soundtrack from the
	// media library is required — otherwise there is nothing to fit it to.
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
			log.Printf("api: get media asset %s: %v", req.AudioAssetID, err)
			writeError(w, http.StatusInternalServerError, "could not load audio")
			return
		}
		if asset == nil {
			writeError(w, http.StatusBadRequest, "выбранный mp3 не найден")
			return
		}
		audioObject = asset.Object
	}

	// sensible defaults for placeholder keys
	firstKey := orDefault(req.FirstNameKey, "firstName")
	lastKey := orDefault(req.LastNameKey, "lastName")
	fullKey := orDefault(req.FullNameKey, "name")

	batch := &store.Batch{
		Title:      strings.TrimSpace(req.Title),
		TemplateID: req.TemplateID,
		VideoModel: req.VideoModel,
	}

	tasks := make([]*store.Task, 0, len(req.Names))
	for _, n := range req.Names {
		first := strings.TrimSpace(n.FirstName)
		last := strings.TrimSpace(n.LastName)
		if first == "" && last == "" {
			continue
		}
		settings := map[string]string{}
		for k, v := range req.ExtraSettings {
			settings[k] = v
		}
		settings[firstKey] = first
		settings[lastKey] = last
		settings[fullKey] = strings.TrimSpace(first + " " + last)

		tasks = append(tasks, &store.Task{
			FirstName:        first,
			LastName:         last,
			ImageSettings:    settings,
			VideoModel:       req.VideoModel,
			VideoPrompt:      req.VideoPrompt,
			VideoDuration:    req.VideoDuration,
			VideoResolution:  req.VideoResolution,
			VideoAspectRatio: req.VideoAspectRatio,
			GenerateAudio:    req.GenerateAudio,
			AudioAssetID:     req.AudioAssetID,
			AudioObject:      audioObject,
		})
	}
	if len(tasks) == 0 {
		writeError(w, http.StatusBadRequest, "no valid names provided")
		return
	}

	if err := s.store.CreateBatch(r.Context(), batch, tasks); err != nil {
		log.Printf("api: create batch: %v", err)
		writeError(w, http.StatusInternalServerError, "could not create batch")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"batchId": batch.ID,
		"count":   len(tasks),
	})
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	batchID := r.URL.Query().Get("batch_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	tasks, err := s.store.ListTasks(r.Context(), batchID, limit)
	if err != nil {
		log.Printf("api: list tasks: %v", err)
		writeError(w, http.StatusInternalServerError, "could not list tasks")
		return
	}
	rate := s.rater.Rate(r.Context())
	// attach presigned download URLs for finished videos + RUB cost
	for _, t := range tasks {
		t.CostRUB = t.CostUSD * rate
		if t.VideoObject == "" {
			continue
		}
		url, err := s.storage.PresignedURL(r.Context(), t.VideoObject, 24*time.Hour)
		if err != nil {
			log.Printf("api: presign %s: %v", t.VideoObject, err)
			continue
		}
		t.VideoURL = url

		dl, err := s.storage.PresignedDownloadURL(r.Context(), t.VideoObject, videoFileName(t), 24*time.Hour)
		if err != nil {
			log.Printf("api: presign download %s: %v", t.VideoObject, err)
			continue
		}
		t.DownloadURL = dl
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks, "usdRubRate": rate})
}

// handleRetryTask puts one failed task back into the pipeline. It resumes from
// the furthest artifact already produced, so an existing OpenRouter job is polled
// again rather than paid for twice.
func (s *Server) handleRetryTask(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "task id is required")
		return
	}
	status, err := s.store.RetryTask(r.Context(), id)
	if err != nil {
		log.Printf("api: retry task %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "could not retry task")
		return
	}
	if status == "" {
		writeError(w, http.StatusConflict, "task not found or not in a failed state")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": status})
}

// handleRetryBatch re-queues every failed task of a batch.
func (s *Server) handleRetryBatch(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "batch id is required")
		return
	}
	n, err := s.store.RetryFailedInBatch(r.Context(), id)
	if err != nil {
		log.Printf("api: retry batch %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "could not retry batch")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"retried": n})
}

func (s *Server) handleListBatches(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	batches, err := s.store.ListBatches(r.Context(), limit)
	if err != nil {
		log.Printf("api: list batches: %v", err)
		writeError(w, http.StatusInternalServerError, "could not list batches")
		return
	}
	rate := s.rater.Rate(r.Context())
	for _, b := range batches {
		b.CostRUB = b.CostUSD * rate
	}
	writeJSON(w, http.StatusOK, map[string]any{"batches": batches, "usdRubRate": rate})
}

func (s *Server) handleGetBatch(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "batch id is required")
		return
	}
	b, err := s.store.GetBatch(r.Context(), id)
	if err != nil {
		log.Printf("api: get batch %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "could not load batch")
		return
	}
	if b == nil {
		writeError(w, http.StatusNotFound, "batch not found")
		return
	}
	rate := s.rater.Rate(r.Context())
	b.CostRUB = b.CostUSD * rate
	writeJSON(w, http.StatusOK, map[string]any{"batch": b, "usdRubRate": rate})
}

func (s *Server) handleDeleteBatch(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "batch id is required")
		return
	}
	exists, err := s.store.BatchExists(r.Context(), id)
	if err != nil {
		log.Printf("api: batch exists %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "could not delete batch")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "batch not found")
		return
	}
	// Best-effort removal of the stored videos for this batch.
	if err := s.storage.RemovePrefix(r.Context(), id+"/"); err != nil {
		log.Printf("api: remove objects for batch %s: %v", id, err)
	}
	if err := s.store.DeleteBatch(r.Context(), id); err != nil {
		log.Printf("api: delete batch %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "could not delete batch")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

// ---------- helpers ----------

func decode(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
