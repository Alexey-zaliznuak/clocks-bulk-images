package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to the OpenRouter video generation API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New builds a client. If proxyURL is non-empty, all OpenRouter traffic is
// routed through that proxy (e.g. "http://user:pass@host:port") — useful when
// the host region is blocked by OpenRouter's edge. timeout bounds a single
// HTTP request; if zero a sane default is used.
func New(baseURL, apiKey, proxyURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	httpClient := &http.Client{Timeout: timeout}
	if proxyURL != "" {
		p, err := url.Parse(proxyURL)
		if err != nil {
			log.Printf("openrouter: invalid OPENROUTER_PROXY_URL %q: %v (proxy disabled)", proxyURL, err)
		} else {
			httpClient.Transport = &http.Transport{Proxy: http.ProxyURL(p)}
			log.Printf("openrouter: routing traffic through proxy %s", p.Host)
		}
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    httpClient,
	}
}

type frameImageURL struct {
	URL string `json:"url"`
}

type frameImage struct {
	ImageURL  frameImageURL `json:"image_url"`
	Type      string        `json:"type"`       // always "image_url"
	FrameType string        `json:"frame_type"` // "first_frame" | "last_frame"
}

type createVideoRequest struct {
	Model        string       `json:"model"`
	Prompt       string       `json:"prompt"`
	FrameImages  []frameImage `json:"frame_images,omitempty"`
	Duration     *int         `json:"duration,omitempty"`
	Resolution   string       `json:"resolution,omitempty"`
	AspectRatio  string       `json:"aspect_ratio,omitempty"`
}

// VideoJob mirrors the relevant fields of a video generation job.
type VideoJob struct {
	ID           string   `json:"id"`
	Status       string   `json:"status"`
	PollingURL   string   `json:"polling_url"`
	UnsignedURLs []string `json:"unsigned_urls"`
	Error        string   `json:"error"`
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
	reqBody := createVideoRequest{
		Model:       p.Model,
		Prompt:      p.Prompt,
		Duration:    p.Duration,
		Resolution:  p.Resolution,
		AspectRatio: p.AspectRatio,
	}
	if p.ImageURL != "" {
		reqBody.FrameImages = []frameImage{{
			ImageURL:  frameImageURL{URL: p.ImageURL},
			Type:      "image_url",
			FrameType: "first_frame",
		}}
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/videos", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	return c.do(req)
}

// GetVideo fetches the current state of a job for polling.
func (c *Client) GetVideo(ctx context.Context, id string) (*VideoJob, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/videos/"+id, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	return c.do(req)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/videos/models", nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openrouter models: status %d: %s", resp.StatusCode, string(data))
	}
	var wrap struct {
		Data []Model `json:"data"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return nil, fmt.Errorf("openrouter: decode models: %w", err)
	}
	return wrap.Data, nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}

func (c *Client) do(req *http.Request) (*VideoJob, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openrouter %s: status %d: %s", req.URL.Path, resp.StatusCode, string(data))
	}
	var j VideoJob
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("openrouter: decode response: %w (body=%s)", err, string(data))
	}
	return &j, nil
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
