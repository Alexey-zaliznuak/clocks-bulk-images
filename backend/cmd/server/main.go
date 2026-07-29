package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"named_clocks/backend/internal/api"
	"named_clocks/backend/internal/auth"
	"named_clocks/backend/internal/config"
	"named_clocks/backend/internal/currency"
	"named_clocks/backend/internal/db"
	"named_clocks/backend/internal/imanator"
	"named_clocks/backend/internal/media"
	"named_clocks/backend/internal/openrouter"
	"named_clocks/backend/internal/storage"
	"named_clocks/backend/internal/store"
	"named_clocks/backend/internal/worker"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- Database ---
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("db migrate: %v", err)
	}
	log.Printf("db: connected and migrated")

	st := store.New(pool)

	// --- Object storage ---
	strg, err := storage.New(ctx, storage.Options{
		Endpoint:       cfg.MinioEndpoint,
		PublicEndpoint: cfg.MinioPublicEndpoint,
		AccessKey:      cfg.MinioAccessKey,
		SecretKey:      cfg.MinioSecretKey,
		Bucket:         cfg.MinioBucket,
		UseSSL:         cfg.MinioUseSSL,
	})
	if err != nil {
		log.Fatalf("storage init: %v", err)
	}
	log.Printf("storage: bucket %q ready", cfg.MinioBucket)

	// --- External clients ---
	imClient := imanator.New(cfg.ImanatorBaseURL, cfg.ImanatorAPIKey)
	orClient := openrouter.New(cfg.OpenRouterBaseURL, cfg.OpenRouterAPIKey, cfg.OpenRouterProxyURL, cfg.OpenRouterTimeout)

	// --- Media tooling ---
	ff := media.New(media.Options{
		Concurrency:      cfg.FFmpegConcurrency,
		TempDir:          cfg.MediaTmpDir,
		SmoothStretch:    cfg.MediaSmoothStretch,
		MaxStretchFactor: cfg.MediaMaxStretchFactor,
		OutputFPS:        cfg.MediaOutputFPS,
	})
	if err := ff.CheckTools(ctx); err != nil {
		// Not fatal: generation without a soundtrack and the rest of the API keep
		// working, but anything media related will fail with a clear message.
		log.Printf("media: WARNING %v — audio mixing and mp3 tools are unavailable", err)
	}

	// --- Worker pool ---
	wk := worker.New(st, imClient, orClient, strg, ff, worker.Options{
		Concurrency:  cfg.WorkerConcurrency,
		PollInterval: cfg.PollInterval,
		StageTimeout: cfg.StageTimeout,
		LeaseTimeout: cfg.LeaseTimeout,
		MaxAttempts:  cfg.MaxTaskAttempts,
	})
	go wk.Run(ctx)

	// --- HTTP API ---
	authn := auth.New(cfg.AppLogin, cfg.AppPassword, cfg.JWTSecret)
	rater := currency.New(cfg.UsdRubRate)
	srv := api.NewServer(st, authn, orClient, strg, ff, rater, api.Options{
		DefaultModel:    cfg.OpenRouterDefaultModel,
		DefaultDuration: cfg.OpenRouterDefaultDuration,
		MaxAudioMB:      cfg.MediaMaxAudioMB,
		MaxVideoMB:      cfg.MediaMaxVideoUploadMB,
	})

	httpServer := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("http: listening on :%s", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}
