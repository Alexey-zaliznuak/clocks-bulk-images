package config

import (
	"log"
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration, loaded from environment variables.
type Config struct {
	HTTPPort string

	AppLogin    string
	AppPassword string
	JWTSecret   string

	DatabaseURL string

	MinioEndpoint       string
	MinioPublicEndpoint string
	MinioAccessKey      string
	MinioSecretKey      string
	MinioBucket         string
	MinioUseSSL         bool

	ImanatorAPIKey  string
	ImanatorBaseURL string

	OpenRouterAPIKey       string
	OpenRouterBaseURL      string
	OpenRouterDefaultModel string
	OpenRouterProxyURL     string
	OpenRouterTimeout      time.Duration

	WorkerConcurrency int
	PollInterval      time.Duration
	StageTimeout      time.Duration
}

// Load reads configuration from the environment, applying sensible defaults.
func Load() *Config {
	c := &Config{
		HTTPPort: env("HTTP_PORT", "8080"),

		AppLogin:    env("APP_LOGIN", "admin"),
		AppPassword: env("APP_PASSWORD", "clocks2026!"),
		JWTSecret:   env("JWT_SECRET", "dev-insecure-secret-change-me"),

		DatabaseURL: env("DATABASE_URL", "postgres://clocks:clocks@localhost:5432/clocks?sslmode=disable"),

		MinioEndpoint:       env("MINIO_ENDPOINT", "localhost:9000"),
		MinioPublicEndpoint: env("MINIO_PUBLIC_ENDPOINT", "localhost:9000"),
		MinioAccessKey:      env("MINIO_ROOT_USER", "minioadmin"),
		MinioSecretKey:      env("MINIO_ROOT_PASSWORD", "minioadmin123"),
		MinioBucket:         env("MINIO_BUCKET", "videos"),
		MinioUseSSL:         envBool("MINIO_USE_SSL", false),

		ImanatorAPIKey:  env("IMANATOR_API_KEY", ""),
		ImanatorBaseURL: env("IMANATOR_BASE_URL", "https://imanator.pro"),

		OpenRouterAPIKey:       env("OPENROUTER_API_KEY", ""),
		OpenRouterBaseURL:      env("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
		OpenRouterDefaultModel: env("OPENROUTER_DEFAULT_MODEL", "google/veo-3.1"),
		OpenRouterProxyURL:     env("OPENROUTER_PROXY_URL", ""),
		OpenRouterTimeout:      time.Duration(envInt("OPENROUTER_TIMEOUT_SECONDS", 120)) * time.Second,

		WorkerConcurrency: envInt("WORKER_CONCURRENCY", 4),
		PollInterval:      time.Duration(envInt("POLL_INTERVAL_SECONDS", 2)) * time.Second,
		StageTimeout:      time.Duration(envInt("STAGE_TIMEOUT_SECONDS", 600)) * time.Second,
	}
	return c
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		log.Printf("config: invalid int for %s=%q, using default %d", key, v, def)
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
