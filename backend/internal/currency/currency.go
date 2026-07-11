package currency

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

// cbrURL is the Central Bank of Russia daily rates feed (no API key required).
const cbrURL = "https://www.cbr-xml-daily.ru/daily_json.js"

// cacheTTL bounds how often we hit the external rate provider.
const cacheTTL = time.Hour

// Rater resolves the current USD→RUB exchange rate. It fetches a live rate from
// the CBR feed, caches it, and falls back to a configured value when the feed
// is unreachable (e.g. the region is blocked or the service is down).
type Rater struct {
	fallback float64
	http     *http.Client

	mu        sync.Mutex
	cached    float64
	fetchedAt time.Time
}

// New builds a Rater with the given fallback rate (used when the live feed
// cannot be reached). If fallback is not positive, 85 is used.
func New(fallback float64) *Rater {
	if fallback <= 0 {
		fallback = 85
	}
	return &Rater{
		fallback: fallback,
		http:     &http.Client{Timeout: 8 * time.Second},
	}
}

// Rate returns the USD→RUB rate, using a cached value when fresh and falling
// back to the configured rate on any failure. It never returns zero.
func (r *Rater) Rate(ctx context.Context) float64 {
	r.mu.Lock()
	if r.cached > 0 && time.Since(r.fetchedAt) < cacheTTL {
		v := r.cached
		r.mu.Unlock()
		return v
	}
	r.mu.Unlock()

	live, err := r.fetch(ctx)
	if err != nil || live <= 0 {
		if err != nil {
			log.Printf("currency: live rate unavailable (%v), using fallback %.2f", err, r.fallback)
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.cached > 0 {
			return r.cached // stale but better than nothing
		}
		return r.fallback
	}

	r.mu.Lock()
	r.cached = live
	r.fetchedAt = time.Now()
	r.mu.Unlock()
	return live
}

type cbrResponse struct {
	Valute struct {
		USD struct {
			Value float64 `json:"Value"`
		} `json:"USD"`
	} `json:"Valute"`
}

func (r *Rater) fetch(ctx context.Context) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cbrURL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var out cbrResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.Valute.USD.Value, nil
}
