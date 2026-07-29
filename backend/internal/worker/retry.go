package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"syscall"
	"time"

	"github.com/OpenRouterTeam/go-sdk/models/sdkerrors"

	"named_clocks/backend/internal/imanator"
	"named_clocks/backend/internal/openrouter"
)

// maxConsecutivePollErrors is how many failed status checks in a row a polling
// stage tolerates before giving up and letting the task be rescheduled.
const maxConsecutivePollErrors = 10

// transientError marks a failure that is expected to go away on its own — a
// network blip, a 5xx, a poll timeout. The task keeps its status and is retried
// from the same stage after a backoff instead of being marked failed.
type transientError struct{ err error }

func (e *transientError) Error() string { return e.err.Error() }
func (e *transientError) Unwrap() error { return e.err }

// transient wraps err so the worker retries it later.
func transient(err error) error {
	if err == nil {
		return nil
	}
	return &transientError{err: err}
}

// isTransient reports whether a failure is worth retrying. Anything not
// recognised as temporary is treated as permanent, so a genuine misconfiguration
// surfaces immediately instead of being retried for hours.
func isTransient(err error) bool {
	if err == nil {
		return false
	}

	var marked *transientError
	if errors.As(err, &marked) {
		return true
	}

	// Timeouts and dropped connections.
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.EOF) {
		return true
	}
	for _, errno := range []syscall.Errno{
		syscall.ECONNRESET, syscall.ECONNABORTED, syscall.ECONNREFUSED,
		syscall.EPIPE, syscall.EHOSTUNREACH, syscall.ENETUNREACH, syscall.ETIMEDOUT,
	} {
		if errors.Is(err, errno) {
			return true
		}
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	// net.Error also matches *url.Error, i.e. any request that never produced a
	// response (proxy died, TLS handshake failed, connection refused).
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Typed HTTP statuses from our own clients.
	var imErr *imanator.HTTPError
	if errors.As(err, &imErr) {
		return isTransientStatus(imErr.StatusCode)
	}
	var orErr *openrouter.HTTPError
	if errors.As(err, &orErr) {
		return isTransientStatus(orErr.StatusCode)
	}

	// Errors surfaced by the OpenRouter SDK.
	var apiErr *sdkerrors.APIError
	if errors.As(err, &apiErr) {
		return isTransientStatus(apiErr.StatusCode)
	}
	return isTransientSDKError(err)
}

func isTransientStatus(code int) bool {
	switch code {
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return true
	}
	return code >= 500
}

// isTransientSDKError checks the SDK's per-status error types. Only server-side
// and rate-limit conditions are retried; 4xx means our request was wrong.
func isTransientSDKError(err error) bool {
	var (
		timeout     *sdkerrors.RequestTimeoutResponseError
		edgeTimeout *sdkerrors.EdgeNetworkTimeoutResponseError
		rateLimited *sdkerrors.TooManyRequestsResponseError
		internal    *sdkerrors.InternalServerResponseError
		badGateway  *sdkerrors.BadGatewayResponseError
		unavailable *sdkerrors.ServiceUnavailableResponseError
		overloaded  *sdkerrors.ProviderOverloadedResponseError
	)
	return errors.As(err, &timeout) ||
		errors.As(err, &edgeTimeout) ||
		errors.As(err, &rateLimited) ||
		errors.As(err, &internal) ||
		errors.As(err, &badGateway) ||
		errors.As(err, &unavailable) ||
		errors.As(err, &overloaded)
}

// backoff returns how long to wait before retry number `attempt` (1-based):
// 30s, 1m, 2m, 4m, 8m, capped at 10m, plus up to 20% jitter so a batch of tasks
// that failed together does not hammer the API in lockstep.
func backoff(attempt int) time.Duration {
	const (
		base    = 30 * time.Second
		ceiling = 10 * time.Minute
	)
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 10 {
		attempt = 10
	}
	d := base << (attempt - 1)
	if d > ceiling || d <= 0 {
		d = ceiling
	}
	return d + time.Duration(rand.Int63n(int64(d)/5+1))
}

// pollTimeout builds the error returned when a stage waits too long. It is
// transient: the provider may simply be slow, so the stage is resumed later.
func pollTimeout(what, id string, waited time.Duration) error {
	return transient(fmt.Errorf("%s poll timeout for %s after %s", what, id, waited))
}
