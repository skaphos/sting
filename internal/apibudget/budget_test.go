// SPDX-License-Identifier: MIT
package apibudget_test

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/skaphos/sting/internal/apibudget"
)

// stubTransport is a RoundTripper that records how many requests reached it and
// returns a canned response. It is the only way to prove a negative — that a
// rejected request never touched the network.
type stubTransport struct {
	mu       sync.Mutex
	calls    int
	header   http.Header
	err      error
	nilResp  bool
	onCall   func(n int) http.Header
	respCode int
}

func (s *stubTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	s.mu.Lock()
	s.calls++
	n := s.calls
	header := s.header
	s.mu.Unlock()

	if s.err != nil {
		return nil, s.err
	}
	if s.nilResp {
		return nil, errors.New("no response")
	}
	if s.onCall != nil {
		header = s.onCall(n)
	}
	code := s.respCode
	if code == 0 {
		code = http.StatusOK
	}
	resp := &http.Response{
		StatusCode: code,
		Header:     http.Header{},
		Body:       http.NoBody,
	}
	for k, vs := range header {
		for _, v := range vs {
			resp.Header.Add(k, v)
		}
	}
	return resp, nil
}

func (s *stubTransport) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func newRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/o/r/commits", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
}

func TestCountsEachRequest(t *testing.T) {
	t.Parallel()
	stub := &stubTransport{}
	tr := apibudget.NewTransport(stub, 0)

	const n = 7
	for range n {
		if _, err := tr.RoundTrip(newRequest(t)); err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
	}

	if got := tr.Consumed(); got != n {
		t.Errorf("Consumed() = %d, want %d", got, n)
	}
	if got := stub.count(); got != n {
		t.Errorf("delegate saw %d requests, want %d", got, n)
	}
}

// TestCeilingStopsAtTheCeilingAndNotBefore is the enforcement contract: every
// request up to the ceiling must succeed, and the first one past it must fail.
// An off-by-one in either direction is a user-visible bug — one clips a query
// that was within budget, the other lets it overspend.
func TestCeilingStopsAtTheCeilingAndNotBefore(t *testing.T) {
	t.Parallel()
	const ceiling = 3
	stub := &stubTransport{}
	tr := apibudget.NewTransport(stub, ceiling)

	for i := 1; i <= ceiling; i++ {
		if _, err := tr.RoundTrip(newRequest(t)); err != nil {
			t.Fatalf("request %d of %d rejected early: %v", i, ceiling, err)
		}
	}

	_, err := tr.RoundTrip(newRequest(t))
	if !errors.Is(err, apibudget.ErrBudgetExceeded) {
		t.Fatalf("request %d error = %v, want ErrBudgetExceeded", ceiling+1, err)
	}

	// A rejected request must not reach the network, and must not be counted
	// as spend — the report has to stay honest about what was actually used.
	if got := stub.count(); got != ceiling {
		t.Errorf("delegate saw %d requests, want %d (rejected request must not dispatch)", got, ceiling)
	}
	if got := tr.Consumed(); got != ceiling {
		t.Errorf("Consumed() = %d, want %d (a rejected request costs nothing)", got, ceiling)
	}
	if got := tr.Remaining(); got != 0 {
		t.Errorf("Remaining() = %d, want 0", got)
	}
}

func TestCeilingOfOneRejectsSecondRequest(t *testing.T) {
	t.Parallel()
	stub := &stubTransport{}
	tr := apibudget.NewTransport(stub, 1)

	if _, err := tr.RoundTrip(newRequest(t)); err != nil {
		t.Fatalf("first request rejected: %v", err)
	}
	if _, err := tr.RoundTrip(newRequest(t)); !errors.Is(err, apibudget.ErrBudgetExceeded) {
		t.Fatalf("second request error = %v, want ErrBudgetExceeded", err)
	}
}

// TestZeroCeilingIsUncappedButCounted pins the documented meaning of 0: an
// intentional uncapped run still has to report what it spent.
func TestZeroCeilingIsUncappedButCounted(t *testing.T) {
	t.Parallel()
	stub := &stubTransport{}
	tr := apibudget.NewTransport(stub, 0)

	const n = 50
	for range n {
		if _, err := tr.RoundTrip(newRequest(t)); err != nil {
			t.Fatalf("uncapped transport rejected a request: %v", err)
		}
	}

	if got := tr.Consumed(); got != n {
		t.Errorf("Consumed() = %d, want %d", got, n)
	}
	if got := tr.Remaining(); got != math.MaxInt {
		t.Errorf("Remaining() = %d, want math.MaxInt for an uncapped transport", got)
	}
	if got := tr.Report().Ceiling; got != 0 {
		t.Errorf("Report().Ceiling = %d, want 0", got)
	}
}

func TestNegativeCeilingTreatedAsUncapped(t *testing.T) {
	t.Parallel()
	tr := apibudget.NewTransport(&stubTransport{}, -5)
	if got := tr.Ceiling(); got != 0 {
		t.Errorf("Ceiling() = %d, want 0", got)
	}
	if _, err := tr.RoundTrip(newRequest(t)); err != nil {
		t.Errorf("negative ceiling should not reject: %v", err)
	}
}

func TestRemainingCountsDown(t *testing.T) {
	t.Parallel()
	tr := apibudget.NewTransport(&stubTransport{}, 5)

	for want := 5; want > 0; want-- {
		if got := tr.Remaining(); got != want {
			t.Fatalf("Remaining() = %d, want %d", got, want)
		}
		if _, err := tr.RoundTrip(newRequest(t)); err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
	}
	if got := tr.Remaining(); got != 0 {
		t.Errorf("Remaining() = %d, want 0 after exhausting the ceiling", got)
	}
}

func TestRateHeadersCaptured(t *testing.T) {
	t.Parallel()
	reset := time.Date(2026, 7, 25, 14, 32, 0, 0, time.UTC)
	stub := &stubTransport{header: http.Header{
		"X-Ratelimit-Limit":     []string{"5000"},
		"X-Ratelimit-Remaining": []string{"4921"},
		"X-Ratelimit-Reset":     []string{strconv.FormatInt(reset.Unix(), 10)},
	}}
	tr := apibudget.NewTransport(stub, 0)

	if _, err := tr.RoundTrip(newRequest(t)); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

	rep := tr.Report()
	if rep.QuotaLimit != 5000 {
		t.Errorf("QuotaLimit = %d, want 5000", rep.QuotaLimit)
	}
	if rep.QuotaRemaining != 4921 {
		t.Errorf("QuotaRemaining = %d, want 4921", rep.QuotaRemaining)
	}
	if !rep.QuotaResetsAt.Equal(reset) {
		t.Errorf("QuotaResetsAt = %v, want %v", rep.QuotaResetsAt, reset)
	}
	if !tr.QuotaSeen() {
		t.Error("QuotaSeen() = false after a response carrying rate headers")
	}
}

func TestRateHeadersAbsent(t *testing.T) {
	t.Parallel()
	tr := apibudget.NewTransport(&stubTransport{}, 0)
	if _, err := tr.RoundTrip(newRequest(t)); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

	rep := tr.Report()
	if rep.QuotaLimit != 0 || rep.QuotaRemaining != 0 {
		t.Errorf("quota fields = %d/%d, want 0/0 when no headers were sent", rep.QuotaRemaining, rep.QuotaLimit)
	}
	if !rep.QuotaResetsAt.IsZero() {
		t.Errorf("QuotaResetsAt = %v, want zero", rep.QuotaResetsAt)
	}
	if tr.QuotaSeen() {
		t.Error("QuotaSeen() = true with no rate headers")
	}
	// Consumption is still reported: accounting never depends on the provider
	// volunteering quota headers.
	if rep.Consumed != 1 {
		t.Errorf("Consumed = %d, want 1", rep.Consumed)
	}
}

func TestRateHeadersMalformed(t *testing.T) {
	t.Parallel()
	stub := &stubTransport{header: http.Header{
		"X-Ratelimit-Limit":     []string{"not-a-number"},
		"X-Ratelimit-Remaining": []string{""},
		"X-Ratelimit-Reset":     []string{"3.5"},
	}}
	tr := apibudget.NewTransport(stub, 0)

	if _, err := tr.RoundTrip(newRequest(t)); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

	rep := tr.Report()
	if rep.QuotaLimit != 0 || rep.QuotaRemaining != 0 || !rep.QuotaResetsAt.IsZero() {
		t.Errorf("malformed headers must be ignored, got %+v", rep)
	}
	if tr.QuotaSeen() {
		t.Error("QuotaSeen() = true for entirely malformed headers")
	}
}

// TestPartialRateHeadersPreserveEarlierObservation covers the case that would
// otherwise silently erase quota state: a later response that omits the headers
// (a 304, or an enterprise proxy that strips them) must not blank out what an
// earlier response reported.
func TestPartialRateHeadersPreserveEarlierObservation(t *testing.T) {
	t.Parallel()
	stub := &stubTransport{onCall: func(n int) http.Header {
		if n == 1 {
			return http.Header{
				"X-Ratelimit-Limit":     []string{"5000"},
				"X-Ratelimit-Remaining": []string{"4000"},
			}
		}
		return nil // second response carries no rate headers
	}}
	tr := apibudget.NewTransport(stub, 0)

	for range 2 {
		if _, err := tr.RoundTrip(newRequest(t)); err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
	}

	rep := tr.Report()
	if rep.QuotaLimit != 5000 || rep.QuotaRemaining != 4000 {
		t.Errorf("quota = %d of %d, want 4000 of 5000 preserved from the first response",
			rep.QuotaRemaining, rep.QuotaLimit)
	}
}

func TestLatestObservationWins(t *testing.T) {
	t.Parallel()
	stub := &stubTransport{onCall: func(n int) http.Header {
		return http.Header{
			"X-Ratelimit-Limit":     []string{"5000"},
			"X-Ratelimit-Remaining": []string{strconv.Itoa(5000 - n)},
		}
	}}
	tr := apibudget.NewTransport(stub, 0)

	for range 3 {
		if _, err := tr.RoundTrip(newRequest(t)); err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
	}

	if got := tr.Report().QuotaRemaining; got != 4997 {
		t.Errorf("QuotaRemaining = %d, want 4997 (the most recent observation)", got)
	}
}

func TestDelegateErrorIsPropagatedAndStillCounted(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("network is down")
	stub := &stubTransport{err: sentinel}
	tr := apibudget.NewTransport(stub, 10)

	_, err := tr.RoundTrip(newRequest(t))
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want %v", err, sentinel)
	}
	// The request left the process, so it counts: it consumed a slot even
	// though it produced no evidence.
	if got := tr.Consumed(); got != 1 {
		t.Errorf("Consumed() = %d, want 1 (a dispatched request counts even when it fails)", got)
	}
}

func TestNilNextUsesDefaultTransport(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "4999")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr := apibudget.NewTransport(nil, 0)
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if got := tr.Report().QuotaRemaining; got != 4999 {
		t.Errorf("QuotaRemaining = %d, want 4999", got)
	}
}

// TestErrBudgetExceededSurvivesHTTPClientWrapping is what makes the sentinel
// usable in practice: http.Client wraps a RoundTripper error in *url.Error, and
// the collect path branches on errors.Is to turn a budget stop into a partial
// result rather than a failure. If the sentinel stopped unwrapping, budget stops
// would surface as opaque errors and discard gathered evidence.
func TestErrBudgetExceededSurvivesHTTPClientWrapping(t *testing.T) {
	t.Parallel()
	client := &http.Client{Transport: apibudget.NewTransport(&stubTransport{}, 1)}

	resp, err := client.Get("https://api.github.com/x")
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	_ = resp.Body.Close()

	if _, err := client.Get("https://api.github.com/x"); !errors.Is(err, apibudget.ErrBudgetExceeded) {
		t.Fatalf("errors.Is(err, ErrBudgetExceeded) = false through http.Client; err = %v", err)
	}
}

// TestConcurrentSafety runs under -race in CI. The count must be exact and the
// ceiling must never be overshot no matter how the goroutines interleave —
// otherwise a concurrent fan-out could silently spend past the cap.
func TestConcurrentSafety(t *testing.T) {
	t.Parallel()
	const (
		goroutines = 16
		perG       = 25
		ceiling    = 200 // below goroutines*perG (400) so rejection is exercised
	)
	stub := &stubTransport{header: http.Header{
		"X-Ratelimit-Limit":     []string{"5000"},
		"X-Ratelimit-Remaining": []string{"4000"},
	}}
	tr := apibudget.NewTransport(stub, ceiling)

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		accepted int
		rejected int
	)
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perG {
				req, err := http.NewRequest(http.MethodGet, "https://api.github.com/x", nil)
				if err != nil {
					return
				}
				_, err = tr.RoundTrip(req)
				mu.Lock()
				if errors.Is(err, apibudget.ErrBudgetExceeded) {
					rejected++
				} else {
					accepted++
				}
				mu.Unlock()
				// Read the accessors concurrently too, so the race detector
				// sees them interleaved with the writes.
				_ = tr.Remaining()
				_ = tr.Report()
			}
		}()
	}
	wg.Wait()

	if accepted != ceiling {
		t.Errorf("accepted = %d, want exactly %d", accepted, ceiling)
	}
	if rejected != goroutines*perG-ceiling {
		t.Errorf("rejected = %d, want %d", rejected, goroutines*perG-ceiling)
	}
	if got := tr.Consumed(); got != ceiling {
		t.Errorf("Consumed() = %d, want %d", got, ceiling)
	}
	if got := stub.count(); got != ceiling {
		t.Errorf("delegate saw %d requests, want %d — the ceiling was overshot", got, ceiling)
	}
}

// TestUnmeteredRequestsBypassTheCeiling covers the exemption for endpoints the
// provider does not charge for. Counting a free pre-flight check against the
// caller's ceiling would overstate the cost and make a small explicit ceiling
// unusable — it would be spent before any evidence could be gathered.
func TestUnmeteredRequestsBypassTheCeiling(t *testing.T) {
	t.Parallel()
	stub := &stubTransport{header: http.Header{
		"X-Ratelimit-Limit":     []string{"5000"},
		"X-Ratelimit-Remaining": []string{"4321"},
	}}
	tr := apibudget.NewTransport(stub, 2)

	// Many unmetered requests must neither be counted nor rejected.
	for range 10 {
		req := newRequest(t).WithContext(apibudget.Unmetered(context.Background()))
		if _, err := tr.RoundTrip(req); err != nil {
			t.Fatalf("unmetered request rejected: %v", err)
		}
	}
	if got := tr.Consumed(); got != 0 {
		t.Errorf("Consumed() = %d, want 0 for unmetered requests", got)
	}
	if got := tr.Remaining(); got != 2 {
		t.Errorf("Remaining() = %d, want the full ceiling of 2", got)
	}

	// The full ceiling is still available to metered requests afterwards.
	for i := range 2 {
		if _, err := tr.RoundTrip(newRequest(t)); err != nil {
			t.Fatalf("metered request %d rejected: %v", i, err)
		}
	}
	if _, err := tr.RoundTrip(newRequest(t)); !errors.Is(err, apibudget.ErrBudgetExceeded) {
		t.Errorf("error = %v, want ErrBudgetExceeded once the ceiling is spent", err)
	}

	// Rate headers observed on an unmetered response are still recorded: that
	// observation is the whole point of the request.
	if got := tr.Report().QuotaRemaining; got != 4321 {
		t.Errorf("QuotaRemaining = %d, want 4321 captured from the unmetered response", got)
	}
}

func TestMeteredIsTheDefault(t *testing.T) {
	t.Parallel()
	tr := apibudget.NewTransport(&stubTransport{}, 5)
	if _, err := tr.RoundTrip(newRequest(t)); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if got := tr.Consumed(); got != 1 {
		t.Errorf("Consumed() = %d, want 1: requests are metered unless marked otherwise", got)
	}
}
