// Package zaleycash talks to the ZaleyCash API: exchange the secret for a
// short-lived Bearer token, then mint a VK Ads token for a named cabinet.
package zaleycash

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultZaleyTTL = 50 * time.Minute

// Token is a ZaleyCash or VK Ads access token with an absolute expiry.
type Token struct {
	AccessToken string
	ExpiresAt   time.Time
}

// Client calls ZaleyCash. The secret is only used for POST /api/v2/token.
type Client struct {
	baseURL string
	secret  string
	http    *http.Client
}

func New(baseURL, secret string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		secret:  secret,
		http:    &http.Client{Timeout: timeout},
	}
}

// AccessToken exchanges the secret key for a ZaleyCash API token (~1 hour).
func (c *Client) AccessToken(ctx context.Context) (Token, error) {
	return c.postToken(ctx, "/api/v2/token", c.secret, nil)
}

// VKAdvertToken mints a VK Ads token for the cabinet whose title is accountName.
// Support requires the name here, not the numeric ZaleyCash id.
func (c *Client) VKAdvertToken(ctx context.Context, zaleyAccessToken, accountName string) (Token, error) {
	name := strings.TrimSpace(accountName)
	if name == "" {
		return Token{}, fmt.Errorf("zaleycash: account name is empty")
	}
	return c.postToken(ctx, "/api/v2/vk_advert/token", zaleyAccessToken, map[string]string{
		"account_id": name,
	})
}

type envelope struct {
	Code     int             `json:"code"`
	Message  string          `json:"message"`
	Response json.RawMessage `json:"response"`
}

type tokenBody struct {
	AccessTokenSnake string `json:"access_token"`
	AccessTokenCamel string `json:"accessToken"`
	ExpiresAtSnake   int64  `json:"expires_at"`
	ExpiresAtCamel   int64  `json:"expiresAt"`
}

func (t tokenBody) accessToken() string {
	if t.AccessTokenSnake != "" {
		return t.AccessTokenSnake
	}
	return strings.TrimSpace(t.AccessTokenCamel)
}

func (t tokenBody) expiresAt() int64 {
	if t.ExpiresAtSnake != 0 {
		return t.ExpiresAtSnake
	}
	return t.ExpiresAtCamel
}

func (c *Client) postToken(ctx context.Context, path, bearer string, body map[string]string) (Token, error) {
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			return Token{}, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Token{}, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		if resp.StatusCode >= 400 {
			return Token{}, &HTTPError{StatusCode: resp.StatusCode, Path: path, Body: string(data)}
		}
		return Token{}, fmt.Errorf("zaleycash %s: decode: %w (body=%s)", path, err, string(data))
	}
	if resp.StatusCode >= 400 {
		msg := env.Message
		if msg == "" {
			msg = string(data)
		}
		return Token{}, &HTTPError{StatusCode: resp.StatusCode, Path: path, Body: msg, Code: env.Code}
	}
	if env.Code != 0 && env.Code != 200 {
		return Token{}, &HTTPError{StatusCode: resp.StatusCode, Path: path, Body: env.Message, Code: env.Code}
	}

	var tok tokenBody
	if err := json.Unmarshal(env.Response, &tok); err != nil {
		return Token{}, fmt.Errorf("zaleycash %s: decode response: %w (body=%s)", path, err, string(data))
	}
	access := tok.accessToken()
	if access == "" {
		return Token{}, fmt.Errorf("zaleycash %s: empty access_token (body=%s)", path, string(data))
	}
	expiresUnix := tok.expiresAt()
	expires := time.Unix(expiresUnix, 0)
	if expiresUnix == 0 {
		expires = time.Now().Add(defaultZaleyTTL)
	}
	return Token{AccessToken: access, ExpiresAt: expires}, nil
}

// HTTPError is a ZaleyCash API failure, either an HTTP status or an envelope code.
type HTTPError struct {
	StatusCode int
	Code       int
	Path       string
	Body       string
}

func (e *HTTPError) Error() string {
	if e.Code != 0 && e.Code != 200 {
		return fmt.Sprintf("zaleycash %s: code %d: %s", e.Path, e.Code, e.Body)
	}
	return fmt.Sprintf("zaleycash %s: status %d: %s", e.Path, e.StatusCode, e.Body)
}
