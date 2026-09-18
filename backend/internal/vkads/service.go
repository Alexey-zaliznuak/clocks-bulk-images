// Package vkads keeps a live VK Ads access token, refreshing it through
// ZaleyCash when the cached value is close to expiry or VK returns 401.
package vkads

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"named_clocks/backend/internal/zaleycash"
)

const (
	defaultSkew     = 2 * time.Minute
	catalogTTL      = time.Hour
	vkMinRequestGap = 400 * time.Millisecond
)

// Service mints and caches the VK Ads token for one ZaleyCash cabinet, then
// signs requests to ads.vk.com with it.
type Service struct {
	zaley   *zaleycash.Client
	account string
	adsURL  string
	skew    time.Duration
	http    *http.Client
	now     func() time.Time

	mu         sync.Mutex
	zaleyTok   cached
	vkTok      cached
	segments   []Segment
	segmentsAt time.Time
	packages   []Package
	packagesAt time.Time
	regions    []Region
	regionsAt  time.Time
	pads       []PadNode
	padsAt     time.Time
	nextOK     time.Time
}

type cached struct {
	token     string
	expiresAt time.Time
}

func (c cached) valid(now time.Time, skew time.Duration) bool {
	return c.token != "" && now.Add(skew).Before(c.expiresAt)
}

// Config wires ZaleyCash to a single VK Ads cabinet.
type Config struct {
	Zaley       *zaleycash.Client
	AccountName string
	AdsBaseURL  string
	RefreshSkew time.Duration
	HTTPTimeout time.Duration
	Now         func() time.Time
}

// New returns nil when the secret client or cabinet name is missing so the rest
// of the app can start without VK Ads.
func New(cfg Config) *Service {
	account := strings.TrimSpace(cfg.AccountName)
	if cfg.Zaley == nil || account == "" {
		return nil
	}
	skew := cfg.RefreshSkew
	if skew <= 0 {
		skew = defaultSkew
	}
	timeout := cfg.HTTPTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		zaley:   cfg.Zaley,
		account: account,
		adsURL:  strings.TrimRight(cfg.AdsBaseURL, "/"),
		skew:    skew,
		http:    &http.Client{Timeout: timeout},
		now:     now,
	}
}

func (s *Service) AccountName() string { return s.account }

// AccessToken returns a VK Ads Bearer token, refreshing through ZaleyCash when
// the cache is missing or about to expire.
func (s *Service) AccessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	if s.vkTok.valid(s.now(), s.skew) {
		tok := s.vkTok.token
		s.mu.Unlock()
		return tok, nil
	}
	s.mu.Unlock()
	return s.refreshVK(ctx)
}

// Invalidate drops the cached VK token so the next call hits ZaleyCash again.
func (s *Service) Invalidate() {
	s.mu.Lock()
	s.vkTok = cached{}
	s.mu.Unlock()
}

func (s *Service) refreshVK(ctx context.Context) (string, error) {
	zaleyTok, err := s.zaleyAccess(ctx)
	if err != nil {
		return "", err
	}
	vk, err := s.zaley.VKAdvertToken(ctx, zaleyTok, s.account)
	if err != nil {
		s.invalidateZaley()
		zaleyTok, retryErr := s.zaleyAccess(ctx)
		if retryErr != nil {
			return "", err
		}
		vk, err = s.zaley.VKAdvertToken(ctx, zaleyTok, s.account)
		if err != nil {
			return "", err
		}
	}
	s.mu.Lock()
	s.vkTok = cached{token: vk.AccessToken, expiresAt: vk.ExpiresAt}
	s.mu.Unlock()
	return vk.AccessToken, nil
}

func (s *Service) zaleyAccess(ctx context.Context) (string, error) {
	s.mu.Lock()
	if s.zaleyTok.valid(s.now(), s.skew) {
		tok := s.zaleyTok.token
		s.mu.Unlock()
		return tok, nil
	}
	s.mu.Unlock()

	tok, err := s.zaley.AccessToken(ctx)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.zaleyTok = cached{token: tok.AccessToken, expiresAt: tok.ExpiresAt}
	s.mu.Unlock()
	return tok.AccessToken, nil
}

func (s *Service) invalidateZaley() {
	s.mu.Lock()
	s.zaleyTok = cached{}
	s.mu.Unlock()
}

// User is the current VK Ads account behind the cached token.
type User struct {
	ID       json.Number `json:"id"`
	Username string      `json:"username"`
	Types    []string    `json:"types"`
}

// CurrentUser checks that the token can sign ads.vk.com requests.
func (s *Service) CurrentUser(ctx context.Context) (*User, error) {
	data, err := s.Get(ctx, "/api/v2/user.json")
	if err != nil {
		return nil, err
	}
	return decodeUser(data)
}

func decodeUser(data []byte) (*User, error) {
	var u User
	if err := json.Unmarshal(data, &u); err == nil && userFilled(u) {
		return &u, nil
	}
	var wrap struct {
		User User `json:"user"`
	}
	if err := json.Unmarshal(data, &wrap); err == nil && userFilled(wrap.User) {
		return &wrap.User, nil
	}
	return nil, fmt.Errorf("vkads: unexpected user.json: %s", truncate(data, 300))
}

func userFilled(u User) bool {
	return u.Username != "" || u.ID.String() != ""
}

// Get sends an authenticated GET to ads.vk.com and retries once after a 401
// with a freshly minted token.
func (s *Service) Get(ctx context.Context, path string) ([]byte, error) {
	return s.do(ctx, http.MethodGet, path, nil)
}

func (s *Service) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var unauthorized bool
	for attempt := 0; attempt < 5; attempt++ {
		data, status, err := s.roundTrip(ctx, method, path, body)
		if err != nil {
			var httpErr *HTTPError
			if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusTooManyRequests && attempt < 4 {
				if err := sleepCtx(ctx, time.Second); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}
		if status != http.StatusUnauthorized {
			return data, nil
		}
		if unauthorized {
			return nil, &HTTPError{StatusCode: status, Path: path, Body: string(data)}
		}
		unauthorized = true
		s.Invalidate()
	}
	return nil, fmt.Errorf("vkads %s: retries exhausted", path)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (s *Service) throttle(ctx context.Context) error {
	s.mu.Lock()
	wait := s.nextOK.Sub(s.now())
	if wait < 0 {
		wait = 0
	}
	s.nextOK = s.now().Add(vkMinRequestGap)
	s.mu.Unlock()
	if wait == 0 {
		return nil
	}
	return sleepCtx(ctx, wait)
}

func (s *Service) roundTrip(ctx context.Context, method, path string, body []byte) ([]byte, int, error) {
	if err := s.throttle(ctx); err != nil {
		return nil, 0, err
	}
	tok, err := s.AccessToken(ctx)
	if err != nil {
		return nil, 0, err
	}
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.adsURL+path, rdr)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusUnauthorized {
		return nil, resp.StatusCode, &HTTPError{StatusCode: resp.StatusCode, Path: path, Body: string(data)}
	}
	return data, resp.StatusCode, nil
}

// HTTPError is a non-2xx response from VK Ads.
type HTTPError struct {
	StatusCode int
	Path       string
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("vkads %s: status %d: %s", e.Path, e.StatusCode, e.Body)
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
