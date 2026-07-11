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
	"named_clocks/backend/internal/openrouter"
	"named_clocks/backend/internal/storage"
	"named_clocks/backend/internal/store"
)

type Server struct {
	store        *store.Store
	auth         *auth.Authenticator
	openrouter   *openrouter.Client
	storage      *storage.Storage
	rater        *currency.Rater
	defaultModel string
	defaultPrompt string
}

// DefaultVideoPrompt is the out-of-the-box prompt for animating the clock image.
const DefaultVideoPrompt = "оживи картинку, рука должна плавно и естественно двигаться, показывая часы с разных сторон. Музыка спокойная, надписи на циферблате строго без искажений и изменений. Секундная стрелка двигается медленно реалистично, строго по часовой стороне."

func NewServer(st *store.Store, a *auth.Authenticator, or *openrouter.Client, strg *storage.Storage, rater *currency.Rater, defaultModel string) *Server {
	return &Server{
		store:         st,
		auth:          a,
		openrouter:    or,
		storage:       strg,
		rater:         rater,
		defaultModel:  defaultModel,
		defaultPrompt: DefaultVideoPrompt,
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
		pr.Get("/api/models", s.handleModels)
		pr.Post("/api/tasks/batch", s.handleCreateBatch)
		pr.Get("/api/tasks", s.handleListTasks)
		pr.Get("/api/batches", s.handleListBatches)
		pr.Delete("/api/batches/{id}", s.handleDeleteBatch)
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
	writeJSON(w, http.StatusOK, map[string]any{
		"defaultModel":  s.defaultModel,
		"defaultPrompt": s.defaultPrompt,
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
	Title            string            `json:"title"`
	TemplateID       string            `json:"templateId"`
	VideoModel       string            `json:"videoModel"`
	VideoPrompt      string            `json:"videoPrompt"`
	VideoDuration    *int              `json:"videoDuration"`
	VideoResolution  string            `json:"videoResolution"`
	VideoAspectRatio string            `json:"videoAspectRatio"`
	ExtraSettings    map[string]string `json:"extraSettings"`
	FirstNameKey     string            `json:"firstNameKey"`
	LastNameKey      string            `json:"lastNameKey"`
	FullNameKey      string            `json:"fullNameKey"`
	Names            []nameInput       `json:"names"`
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
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks, "usdRubRate": rate})
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
