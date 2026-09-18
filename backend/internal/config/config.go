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
	// OpenRouterDefaultDuration is the clip length in seconds requested when the
	// user leaves the duration empty.
	OpenRouterDefaultDuration int
	OpenRouterProxyURL        string
	OpenRouterTimeout         time.Duration

	WorkerConcurrency int
	PollInterval      time.Duration
	// StageTimeout bounds how long a single stage may wait for an external
	// provider before the task is rescheduled.
	StageTimeout time.Duration
	// LeaseTimeout is how long a claimed task stays locked without being
	// touched. It also decides how fast work is picked up again after the
	// process dies mid-task, so keep it well below StageTimeout.
	LeaseTimeout time.Duration
	// MaxTaskAttempts is the total number of tries per task (first attempt plus
	// retries) before a transient failure is treated as final.
	MaxTaskAttempts int

	// FFmpegConcurrency caps simultaneous ffmpeg processes (encoding is CPU
	// bound, so it should stay below WorkerConcurrency).
	FFmpegConcurrency int
	// MediaTmpDir holds scratch files for uploads and encodes.
	MediaTmpDir string
	// MediaStretchMode is how a clip is retimed to the soundtrack length:
	// "interpolate" synthesises the missing frames, "duplicate" holds the
	// existing ones longer (cheap, but visibly stuttery).
	MediaStretchMode string
	// MediaMaxStretchFactor is the largest slow-down allowed when fitting a clip
	// to a soundtrack.
	MediaMaxStretchFactor float64
	// MediaOutputFPS is the frame rate of the rendered videos.
	MediaOutputFPS int
	// MediaFFmpegTimeout bounds a single ffmpeg run. Interpolated renders take
	// minutes, and proportionally longer on a small machine.
	MediaFFmpegTimeout time.Duration
	// MediaMaxAudioMB / MediaMaxVideoUploadMB bound the upload endpoints.
	MediaMaxAudioMB       int64
	MediaMaxVideoUploadMB int64

	// UsdRubRate is the fallback USD→RUB rate used when the live feed is
	// unavailable. A live rate is still preferred when reachable.
	UsdRubRate float64

	// ZaleySecret is the API key from ZaleyCash settings. Empty disables VK Ads.
	ZaleySecret string
	// ZaleyAccountName is the cabinet title passed as account_id to
	// POST /api/v2/vk_advert/token (the name, not the numeric id).
	ZaleyAccountName string
	ZaleyBaseURL     string
	VKAdsBaseURL     string
	// ZaleyTokenRefreshSkew is how early a cached token is treated as expired.
	ZaleyTokenRefreshSkew time.Duration
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

		OpenRouterAPIKey:          env("OPENROUTER_API_KEY", ""),
		OpenRouterBaseURL:         env("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
		OpenRouterDefaultModel:    env("OPENROUTER_DEFAULT_MODEL", "google/veo-3.1-lite"),
		OpenRouterDefaultDuration: envInt("OPENROUTER_DEFAULT_DURATION", 4),
		OpenRouterProxyURL:        env("OPENROUTER_PROXY_URL", ""),
		OpenRouterTimeout:         time.Duration(envInt("OPENROUTER_TIMEOUT_SECONDS", 120)) * time.Second,

		WorkerConcurrency: envInt("WORKER_CONCURRENCY", 8),
		PollInterval:      time.Duration(envInt("POLL_INTERVAL_SECONDS", 2)) * time.Second,
		StageTimeout:      time.Duration(envInt("STAGE_TIMEOUT_SECONDS", 1800)) * time.Second,
		LeaseTimeout:      time.Duration(envInt("LEASE_TIMEOUT_SECONDS", 90)) * time.Second,
		MaxTaskAttempts:   envInt("MAX_TASK_ATTEMPTS", 5),

		FFmpegConcurrency:     envInt("FFMPEG_CONCURRENCY", 3),
		MediaTmpDir:           env("MEDIA_TMP_DIR", ""),
		MediaStretchMode:      env("MEDIA_STRETCH_MODE", "interpolate"),
		MediaMaxStretchFactor: envFloat("MEDIA_MAX_STRETCH_FACTOR", 6),
		MediaOutputFPS:        envInt("MEDIA_OUTPUT_FPS", 30),
		MediaFFmpegTimeout:    time.Duration(envInt("MEDIA_FFMPEG_TIMEOUT_SECONDS", 1800)) * time.Second,
		MediaMaxAudioMB:       int64(envInt("MEDIA_MAX_AUDIO_MB", 50)),
		MediaMaxVideoUploadMB: int64(envInt("MEDIA_MAX_VIDEO_UPLOAD_MB", 500)),

		UsdRubRate: envFloat("USD_RUB_RATE", 85),

		ZaleySecret:           env("ZALEY_SECRET", ""),
		ZaleyAccountName:      env("ZALEY_ACCOUNT_NAME", ""),
		ZaleyBaseURL:          env("ZALEY_BASE_URL", "https://zaleycash.com"),
		VKAdsBaseURL:          env("VK_ADS_BASE_URL", "https://ads.vk.com"),
		ZaleyTokenRefreshSkew: time.Duration(envInt("ZALEY_TOKEN_REFRESH_SKEW_SECONDS", 120)) * time.Second,
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

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
		log.Printf("config: invalid float for %s=%q, using default %g", key, v, def)
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
