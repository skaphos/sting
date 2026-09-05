// SPDX-License-Identifier: MIT
package ghclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

var prClock = time.Date(2026, 8, 25, 13, 0, 0, 0, time.UTC)
var prReadPath = regexp.MustCompile(`^/(user|search/issues|orgs/[^/]+/repos|repos/[^/]+/[^/]+/(pulls|issues/[0-9]+/events))$`)

type prFixture struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []string
}

func newPRFixture(t *testing.T, handler http.HandlerFunc) *prFixture {
	t.Helper()
	f := &prFixture{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.RequestURI())
		f.mu.Unlock()
		r = r.Clone(r.Context())
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api/v3")
		if r.Method != http.MethodGet || !prReadPath.MatchString(r.URL.Path) {
			t.Errorf("unexpected provider request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected endpoint", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	}))
	t.Cleanup(f.server.Close)
	return f
}
func (f *prFixture) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.requests) }
func prJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Error(err)
	}
}
func prPayload(n int) map[string]any {
	return map[string]any{
		"number": n, "html_url": "https://github.com/acme/api/pull/1", "title": "Example", "state": "open", "draft": false,
		"user": map[string]any{"login": "octocat"}, "created_at": "2026-08-24T13:00:00Z", "updated_at": "2026-08-24T13:00:00Z",
		"merged_at": nil, "closed_at": nil, "assignees": []any{}, "requested_reviewers": []any{}, "requested_teams": []any{}, "labels": []any{},
		"base": map[string]any{"ref": "main"}, "head": map[string]any{"ref": "feature"}, "milestone": nil,
	}
}
