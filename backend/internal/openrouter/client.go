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
	return c.doRequest(ctx, http.MethodPost, c.baseURL+"/videos", body)
}

// GetVideo fetches the current state of a job for polling.
func (c *Client) GetVideo(ctx context.Context, id string) (*VideoJob, error) {
	return c.doRequest(ctx, http.MethodGet, c.baseURL+"/videos/"+id, nil)
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

// maxAttempts bounds how many times a request is retried on transient failures
// (flaky proxy / dropped connections / 5xx / 429).
const maxAttempts = 4

// doRequest executes a request with retries on transient errors. body may be nil.
func (c *Client) doRequest(ctx context.Context, method, urlStr string, body []byte) (*VideoJob, error) {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, urlStr, bodyReader)
		if err != nil {
			return nil, err
		}
		c.setHeaders(req)

		j, retryable, err := c.attempt(req)
		if err == nil {
			return j, nil
		}
		lastErr = err
		if !retryable || attempt == maxAttempts || ctx.Err() != nil {
			break
		}
		backoff := time.Duration(attempt) * 2 * time.Second
		log.Printf("openrouter: %s %s attempt %d/%d failed: %v (retrying in %s)", method, req.URL.Path, attempt, maxAttempts, err, backoff)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
	}
	return nil, lastErr
}

// attempt performs a single HTTP call. The bool reports whether the error is
// worth retrying.
func (c *Client) attempt(req *http.Request) (*VideoJob, bool, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		// Network-level failures (EOF, reset, timeout, proxy drop) are transient.
		return nil, true, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return nil, retryable, fmt.Errorf("openrouter %s: status %d: %s", req.URL.Path, resp.StatusCode, string(data))
	}
	var j VideoJob
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, false, fmt.Errorf("openrouter: decode response: %w (body=%s)", err, string(data))
	}
	return &j, false, nil
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
