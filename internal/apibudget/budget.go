// SPDX-License-Identifier: MIT
// Package apibudget accounts for and bounds the provider HTTP requests a query
// consumes. It works at the transport layer rather than at call sites, which
// gives exact counts (including pagination and retries), covers every existing
// provider path without touching one of them, and puts rate-header capture in a
// single place.
//
// The mechanism is internal; only its data shape (model.CostReport) is public,
// keeping the public API minimal.
package apibudget

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/skaphos/sting/model"
)

// ErrBudgetExceeded is returned instead of dispatching a request once the
// configured ceiling has been consumed. Callers branch on it with errors.Is to
// convert a budget stop into a partial result plus a disclosure rather than a
// failure.
//
// It surfaces through http.Client wrapped in a *url.Error, which unwraps, so
// errors.Is keeps working end to end.
var ErrBudgetExceeded = errors.New("api request budget exceeded")

// Transport counts every request it dispatches, refuses to dispatch beyond a
// ceiling, and records the rate-limit headers from every response.
//
// A ceiling of 0 disables the cap but keeps counting: accounting is never
// optional, only enforcement is.
type Transport struct {
	next    http.RoundTripper
	ceiling int

	// consumed counts requests actually dispatched. A request rejected for
	// exceeding the ceiling never reaches the network and is not counted.
	consumed atomic.Int64

	// mu guards the rate-limit observation, which is a small struct rather
	// than independent atomics so a reader never sees a limit from one
	// response paired with a reset time from another.
	mu   sync.Mutex
	rate rateObservation
}

type rateObservation struct {
	limit     int
	remaining int
	resetsAt  time.Time
	seen      bool
}

// NewTransport wraps next, counting and bounding the requests that pass through
// it. A nil next uses http.DefaultTransport. ceiling <= 0 means uncapped.
func NewTransport(next http.RoundTripper, ceiling int) *Transport {
	if next == nil {
		next = http.DefaultTransport
	}
	if ceiling < 0 {
		ceiling = 0
	}
	return &Transport{next: next, ceiling: ceiling}
}

// unmeteredKey marks a request as exempt from the ceiling and from the consumed
// count.
type unmeteredKey struct{}

// Unmetered marks requests made with the returned context as not counting
// against the ceiling.
//
// It exists for endpoints the provider itself does not charge — GitHub's
// /rate_limit is the motivating case. Charging a free pre-flight check against
// the user's request budget would both misstate what the query cost and make a
// small explicit ceiling unusable, since the check would consume it before any
// evidence could be gathered.
//
// Rate headers from an unmetered response are still recorded: the observation
// is exactly what the request was for.
func Unmetered(ctx context.Context) context.Context {
	return context.WithValue(ctx, unmeteredKey{}, true)
}

func isUnmetered(ctx context.Context) bool {
	v, _ := ctx.Value(unmeteredKey{}).(bool)
	return v
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if isUnmetered(req.Context()) {
		resp, err := t.next.RoundTrip(req)
		if resp != nil {
			t.observeRate(resp.Header)
		}
		return resp, err
	}
	return t.meteredRoundTrip(req)
}

func (t *Transport) meteredRoundTrip(req *http.Request) (*http.Response, error) {
	// Reserve capacity before dispatching. On overshoot the reservation is
	// returned, so a rejected request costs nothing and the reported count
	// stays exactly the number of requests that reached the network.
	n := t.consumed.Add(1)
	if t.ceiling > 0 && n > int64(t.ceiling) {
		t.consumed.Add(-1)
		return nil, ErrBudgetExceeded
	}

	resp, err := t.next.RoundTrip(req)
	if resp != nil {
		t.observeRate(resp.Header)
	}
	return resp, err
}

// observeRate records the rate-limit headers from a response. Absent or
// malformed values leave the previous observation intact: a response without
// rate headers (an enterprise proxy, a cached 304) must not erase what an
// earlier response reported.
func (t *Transport) observeRate(h http.Header) {
	if h == nil {
		return
	}
	limit, haveLimit := parseIntHeader(h, "X-RateLimit-Limit")
	remaining, haveRemaining := parseIntHeader(h, "X-RateLimit-Remaining")
	reset, haveReset := parseIntHeader(h, "X-RateLimit-Reset")
	if !haveLimit && !haveRemaining && !haveReset {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if haveLimit {
		t.rate.limit = limit
	}
	if haveRemaining {
		t.rate.remaining = remaining
	}
	if haveReset {
		t.rate.resetsAt = time.Unix(int64(reset), 0).UTC()
	}
	t.rate.seen = true
}

func parseIntHeader(h http.Header, key string) (int, bool) {
	raw := h.Get(key)
	if raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return n, true
}

// Consumed reports how many requests have been dispatched.
func (t *Transport) Consumed() int {
	return int(t.consumed.Load())
}

// Ceiling reports the configured cap; 0 means uncapped.
func (t *Transport) Ceiling() int { return t.ceiling }

// Remaining reports how many more requests the ceiling permits. It returns
// math.MaxInt when uncapped.
//
// This is the check-before-dispatch accessor: a concurrent fan-out asks how
// much it can afford and dispatches only a batch it can fully pay for, rather
// than firing requests and letting the losers fail. Without that, which
// requests won the race would decide which commits got enriched, and the same
// query could return different results run to run.
func (t *Transport) Remaining() int {
	if t.ceiling <= 0 {
		return math.MaxInt
	}
	remaining := int64(t.ceiling) - t.consumed.Load()
	if remaining < 0 {
		return 0
	}
	return int(remaining)
}

// QuotaSeen reports whether any response carried rate-limit headers.
func (t *Transport) QuotaSeen() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.rate.seen
}

// Report snapshots consumption and the latest quota observation. It is safe to
// call at any point, including on an early-return path — a query that stopped
// early must still report what it spent.
func (t *Transport) Report() model.CostReport {
	t.mu.Lock()
	rate := t.rate
	t.mu.Unlock()

	return model.CostReport{
		Consumed:       t.Consumed(),
		Ceiling:        t.ceiling,
		QuotaRemaining: rate.remaining,
		QuotaLimit:     rate.limit,
		QuotaResetsAt:  rate.resetsAt,
	}
}
