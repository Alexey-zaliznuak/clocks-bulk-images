package worker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"syscall"
	"testing"
	"time"

	"github.com/OpenRouterTeam/go-sdk/models/sdkerrors"

	"named_clocks/backend/internal/imanator"
	"named_clocks/backend/internal/openrouter"
	"named_clocks/backend/internal/vkads"
)

func TestIsTransient(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error is permanent", errors.New("boom"), false},
		{"explicitly marked", transient(errors.New("boom")), true},
		{"wrapped marked", fmt.Errorf("stage: %w", transient(errors.New("boom"))), true},
		{"poll timeout", pollTimeout("openrouter", "job-1", time.Minute), true},
		{"deadline exceeded", fmt.Errorf("call: %w", context.DeadlineExceeded), true},
		{"connection reset", fmt.Errorf("read: %w", syscall.ECONNRESET), true},
		{"dns failure", &net.DNSError{Err: "no such host", Name: "openrouter.ai"}, true},
		{
			"proxy died mid-request",
			&url.Error{Op: "Post", URL: "https://openrouter.ai", Err: errors.New("EOF")},
			true,
		},
		{"imanator template missing", &imanator.TemplateNotFoundError{TemplateID: "tpl-1"}, false},
		{"wrapped template missing", fmt.Errorf("imanator create order: %w", &imanator.TemplateNotFoundError{TemplateID: "tpl-1"}), false},
		{"imanator 503", &imanator.HTTPError{StatusCode: 503}, true},
		{"imanator 429", &imanator.HTTPError{StatusCode: 429}, true},
		{"imanator outage 404", &imanator.HTTPError{StatusCode: 404}, true},
		{"imanator html 404", &imanator.HTTPError{StatusCode: 404, Body: "<html>404</html>"}, true},
		{"imanator app route 404", &imanator.HTTPError{
			StatusCode: 404,
			Body:       `{"statusCode":404,"path":"/api/image-generation-orders","message":"Not found"}`,
		}, false},
		{"imanator 400", &imanator.HTTPError{StatusCode: 400}, false},
		{"imanator 401", &imanator.HTTPError{StatusCode: 401}, false},
		{"openrouter download 502", &openrouter.HTTPError{StatusCode: 502}, true},
		{"openrouter download 404", &openrouter.HTTPError{StatusCode: 404}, false},
		{"sdk 500", sdkerrors.NewAPIError("oops", 500, "", nil), true},
		{"sdk 422", sdkerrors.NewAPIError("bad input", 422, "", nil), false},
		{"sdk service unavailable", &sdkerrors.ServiceUnavailableResponseError{}, true},
		{"sdk rate limited", &sdkerrors.TooManyRequestsResponseError{}, true},
		{"sdk unauthorized", &sdkerrors.UnauthorizedResponseError{}, false},
		{"sdk bad request", &sdkerrors.BadRequestResponseError{}, false},
		{"vk ads 503", &vkads.HTTPError{StatusCode: 503}, true},
		{"vk ads 400", &vkads.HTTPError{StatusCode: 400}, false},
		{"audience miss", &vkads.AudienceNotFoundError{Name: "Иван"}, false},
		{
			"wrapped transient status",
			fmt.Errorf("imanator get order: %w", &imanator.HTTPError{StatusCode: 500}),
			true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isTransient(c.err); got != c.want {
				t.Fatalf("isTransient(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

// A permanently failed provider job must not be retried: it would poll a corpse
// until the attempt budget runs out.
func TestFailedJobErrorIsPermanent(t *testing.T) {
	err := fmt.Errorf("openrouter job failed: status=%s %s", "failed", "content policy")
	if isTransient(err) {
		t.Fatal("a failed provider job should be permanent")
	}
}

func TestBackoffGrowsAndIsCapped(t *testing.T) {
	const ceiling = 10 * time.Minute

	prev := time.Duration(0)
	for attempt := 1; attempt <= 4; attempt++ {
		d := backoff(attempt)
		if d <= prev {
			t.Fatalf("backoff(%d) = %s, expected more than previous %s", attempt, d, prev)
		}
		prev = d
	}
	for _, attempt := range []int{6, 10, 50} {
		if d := backoff(attempt); d > ceiling+ceiling/5 {
			t.Fatalf("backoff(%d) = %s, expected at most %s plus jitter", attempt, d, ceiling)
		}
	}
	if d := backoff(1); d < 30*time.Second {
		t.Fatalf("backoff(1) = %s, expected at least the 30s base", d)
	}
	// Guard against a zero or negative wait, which would spin the worker.
	if d := backoff(0); d <= 0 {
		t.Fatalf("backoff(0) = %s, expected a positive delay", d)
	}
}

func TestImanatorRetryDelayIsAlwaysOneMinute(t *testing.T) {
	for _, status := range []string{"queued", "image_creating", "image_polling"} {
		for _, attempt := range []int{1, 5, 100} {
			if got := retryDelay(status, attempt); got != time.Minute {
				t.Fatalf("retryDelay(%q, %d) = %s, want 1m", status, attempt, got)
			}
		}
	}
}

func TestNonImanatorRetryDelayUsesBackoff(t *testing.T) {
	for _, status := range []string{"image_ready", "video_creating", "video_polling", "audio_mixing"} {
		if got := retryDelay(status, 2); got < time.Minute {
			t.Fatalf("retryDelay(%q, 2) = %s, expected exponential backoff", status, got)
		}
	}
}
