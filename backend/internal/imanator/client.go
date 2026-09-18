package imanator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to the Imanator image generation API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Order mirrors the relevant fields of an image generation order.
type Order struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Result   string `json:"result"` // storage URL of the rendered image
	Settings any    `json:"settings"`
}

type createOrderRequest struct {
	TemplateID string            `json:"templateId"`
	Settings   map[string]string `json:"settings"`
}

// CreateOrder submits a new image generation order and returns it.
func (c *Client) CreateOrder(ctx context.Context, templateID string, settings map[string]string) (*Order, error) {
	body, _ := json.Marshal(createOrderRequest{TemplateID: templateID, Settings: settings})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/image-generation-orders", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	order, err := c.do(req)
	if err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, &TemplateNotFoundError{TemplateID: templateID, Err: httpErr}
		}
		return nil, err
	}
	return order, nil
}

// GetOrder fetches the current state of an order for polling.
func (c *Client) GetOrder(ctx context.Context, id string) (*Order, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/image-generation-orders/"+id, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	return c.do(req)
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}

// HTTPError is a non-2xx response from the Imanator API. It carries the status
// code so callers can tell a temporary outage (5xx) from a bad request.
type HTTPError struct {
	StatusCode int
	Path       string
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("imanator %s: status %d: %s", e.Path, e.StatusCode, e.Body)
}

// TemplateNotFoundError is a 404 from create-order: the template id does not exist.
type TemplateNotFoundError struct {
	TemplateID string
	Err        error
}

func (e *TemplateNotFoundError) Error() string {
	return fmt.Sprintf("шаблон Imanator %q не найден", e.TemplateID)
}

func (e *TemplateNotFoundError) Unwrap() error { return e.Err }

// IsApplicationNotFound reports a JSON 404 from the Imanator app itself
// (route or resource missing). An empty or HTML 404 from a dead proxy is not this.
func (e *HTTPError) IsApplicationNotFound() bool {
	if e == nil || e.StatusCode != http.StatusNotFound {
		return false
	}
	body := strings.ToLower(e.Body)
	return strings.Contains(body, `"statuscode":404`) && strings.Contains(body, "not found")
}

func (c *Client) do(req *http.Request) (*Order, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Path: req.URL.Path, Body: string(data)}
	}
	var o Order
	if err := json.Unmarshal(data, &o); err != nil {
		return nil, fmt.Errorf("imanator: decode response: %w (body=%s)", err, string(data))
	}
	return &o, nil
}

// IsReady reports whether the order has produced a downloadable image.
func (o *Order) IsReady() bool {
	if strings.HasPrefix(o.Result, "http") {
		return true
	}
	switch strings.ToLower(o.Status) {
	case "done", "completed", "success", "succeeded", "ready", "finished":
		return o.Result != ""
	}
	return false
}

// IsFailed reports whether the order ended in an error state.
func (o *Order) IsFailed() bool {
	switch strings.ToLower(o.Status) {
	case "failed", "error", "canceled", "cancelled", "rejected":
		return true
	}
	return false
}
