package openrouter

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	orsdk "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/retry"
)

// Client is a thin facade over the official OpenRouter Go SDK, exposing only the
// video-generation surface this app needs.
type Client struct {
	sdk     *orsdk.OpenRouter
	dlHTTP  *http.Client // shares the proxy transport, longer timeout for downloads
	baseURL string
	apiKey  string
}

// New builds a client. If proxyURL is non-empty, all OpenRouter traffic is
// routed through that proxy (e.g. "http://user:pass@host:port") — useful when
// the host region is blocked by OpenRouter's edge. timeout bounds a single
// HTTP attempt; if zero a sane default is used. Transient failures (dropped
// proxy connections / 5xx) are retried by the SDK.
func New(baseURL, apiKey, proxyURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSHandshakeTimeout = 30 * time.Second
	transport.ResponseHeaderTimeout = timeout
	if proxyURL != "" {
		p, err := url.Parse(proxyURL)
		if err != nil {
			log.Printf("openrouter: invalid OPENROUTER_PROXY_URL %q: %v (proxy disabled)", proxyURL, err)
		} else {
			transport.Proxy = http.ProxyURL(p)
			log.Printf("openrouter: routing traffic through proxy %s", p.Host)
		}
	}
	httpClient := &http.Client{Timeout: timeout, Transport: transport}

	opts := []orsdk.SDKOption{
		orsdk.WithClient(httpClient),
		orsdk.WithSecurity(apiKey),
		// Retry dropped connections and 5xx with capped backoff (values in ms).
		orsdk.WithRetryConfig(retry.Config{
			Strategy: "backoff",
			Backoff: &retry.BackoffStrategy{
				InitialInterval: 2000,
				MaxInterval:     15000,
				Exponent:        1.5,
				MaxElapsedTime:  180000,
			},
			RetryConnectionErrors: true,
		}),
	}
	base := strings.TrimRight(baseURL, "/")
	if base != "" {
		opts = append(opts, orsdk.WithServerURL(base))
	}
	return &Client{
		sdk: orsdk.New(opts...),
		// Downloads reuse the proxy transport but need a generous timeout for
		// large video files.
		dlHTTP:  &http.Client{Timeout: 15 * time.Minute, Transport: transport},
		baseURL: base,
		apiKey:  apiKey,
	}
}

// VideoJob mirrors the relevant fields of a video generation job.
type VideoJob struct {
	ID           string
	Status       string
	PollingURL   string
	UnsignedURLs []string
	Error        string
	// Cost is the generation cost in USD, available once the job completes.
	Cost *float64
}

// CreateVideoParams describes an image-to-video (or text-to-video) request.
type CreateVideoParams struct {
	Model       string
	Prompt      string
	ImageURL    string // if set → image-to-video (used as first frame)
	Duration    *int
	Resolution  string
	AspectRatio string
}

// CreateVideo submits a video generation job.
func (c *Client) CreateVideo(ctx context.Context, p CreateVideoParams) (*VideoJob, error) {
	req := components.VideoGenerationRequest{
		Model:  p.Model,
		Prompt: p.Prompt,
	}
	if p.Duration != nil {
		d := int64(*p.Duration)
		req.Duration = &d
	}
	if p.Resolution != "" {
		r := components.Resolution(p.Resolution)
		req.Resolution = &r
	}
	if p.AspectRatio != "" {
		a := components.AspectRatio(p.AspectRatio)
		req.AspectRatio = &a
	}
	if p.ImageURL != "" {
		req.FrameImages = []components.FrameImage{{
			ImageURL:  components.FrameImageImageURL{URL: p.ImageURL},
			Type:      components.FrameImageTypeImageURL,
			FrameType: components.FrameTypeFirstFrame,
		}}
	}
	resp, err := c.sdk.VideoGeneration.Generate(ctx, req)
	if err != nil {
		return nil, err
	}
	return toVideoJob(resp), nil
}

// GetVideo fetches the current state of a job for polling.
func (c *Client) GetVideo(ctx context.Context, id string) (*VideoJob, error) {
	resp, err := c.sdk.VideoGeneration.GetGeneration(ctx, id)
	if err != nil {
		return nil, err
	}
	return toVideoJob(resp), nil
}

// DownloadVideo streams the finished video content from the OpenRouter content
// endpoint through the proxy, with auth. We fetch it manually (rather than via
// the SDK) because the endpoint returns video/* content types and the SDK only
// accepts application/octet-stream, otherwise dumping the whole body into an
// error. The caller must close the returned reader.
func (c *Client) DownloadVideo(ctx context.Context, jobID string, index int) (io.ReadCloser, error) {
	u := fmt.Sprintf("%s/videos/%s/content?index=%d", c.baseURL, url.PathEscape(jobID), index)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.dlHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(data))
	}
	return resp.Body, nil
}

func toVideoJob(r *components.VideoGenerationResponse) *VideoJob {
	if r == nil {
		return nil
	}
	j := &VideoJob{
		ID:           r.ID,
		Status:       string(r.Status),
		PollingURL:   r.PollingURL,
		UnsignedURLs: r.UnsignedUrls,
	}
	if r.Error != nil {
		j.Error = *r.Error
	}
	if r.Usage != nil {
		if v, ok := r.Usage.Cost.Get(); ok && v != nil {
			j.Cost = v
		}
	}
	return j
}

// Model is a minimal view of a video generation model for the frontend.
type Model struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	SupportedResolutions  []string `json:"supported_resolutions"`
	SupportedAspectRatios []string `json:"supported_aspect_ratios"`
	SupportedDurations    []int    `json:"supported_durations"`
}

// ListModels returns all available video generation models.
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	resp, err := c.sdk.VideoGeneration.ListVideosModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Model, 0, len(resp.Data))
	for _, m := range resp.Data {
		mm := Model{ID: m.ID, Name: m.Name}
		for _, r := range m.SupportedResolutions {
			mm.SupportedResolutions = append(mm.SupportedResolutions, string(r))
		}
		for _, a := range m.SupportedAspectRatios {
			mm.SupportedAspectRatios = append(mm.SupportedAspectRatios, string(a))
		}
		for _, d := range m.SupportedDurations {
			mm.SupportedDurations = append(mm.SupportedDurations, int(d))
		}
		out = append(out, mm)
	}
	return out, nil
}

// IsReady reports whether the job produced a downloadable video.
func (j *VideoJob) IsReady() bool {
	return strings.ToLower(j.Status) == "completed" && len(j.UnsignedURLs) > 0
}

// IsFailed reports whether the job ended in an error state.
func (j *VideoJob) IsFailed() bool {
	switch strings.ToLower(j.Status) {
	case "failed", "cancelled", "canceled", "expired":
		return true
	}
	return false
}

// VideoURL returns the first available output URL.
func (j *VideoJob) VideoURL() string {
	if len(j.UnsignedURLs) > 0 {
		return j.UnsignedURLs[0]
	}
	return ""
}
