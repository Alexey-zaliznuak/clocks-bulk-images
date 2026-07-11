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
	"named_clocks/backend/internal/db"
	"named_clocks/backend/internal/imanator"
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
	orClient := openrouter.New(cfg.OpenRouterBaseURL, cfg.OpenRouterAPIKey, cfg.OpenRouterProxyURL)

	// --- Worker pool ---
	wk := worker.New(st, imClient, orClient, strg, cfg.WorkerConcurrency, cfg.PollInterval, cfg.StageTimeout)
	go wk.Run(ctx)

	// --- HTTP API ---
	authn := auth.New(cfg.AppLogin, cfg.AppPassword, cfg.JWTSecret)
	srv := api.NewServer(st, authn, orClient, strg, cfg.OpenRouterDefaultModel)

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
