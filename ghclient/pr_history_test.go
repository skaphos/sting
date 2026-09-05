// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/model"
)

func prEvent(id int, event, at string) map[string]any {
	return map[string]any{"id": id, "event": event, "created_at": at}
}
func TestPRHistoricalSearch(t *testing.T) {
	f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search/issues" {
			query := r.URL.Query().Get("q")
			if !strings.Contains(query, "created:<=") || strings.Contains(query, "updated:") || strings.Contains(query, "is:open") {
				t.Errorf("historical candidate query: %s", query)
			}
			p := prPayload(1)
			p["created_at"] = "2020-01-01T00:00:00Z"
			p["updated_at"] = "2026-09-01T00:00:00Z"
			p["repository_url"] = "https://api.github.com/repos/acme/api"
			p["pull_request"] = map[string]any{}
			prJSON(t, w, map[string]any{"total_count": 1, "incomplete_results": false, "items": []any{p}})
			return
		}
		prJSON(t, w, []any{prEvent(3, "reopened", "2026-09-01T00:00:00Z"), prEvent(2, "closed", "2026-08-24T15:00:00Z"), prEvent(1, "closed", "2026-08-24T15:00:00Z"), prEvent(2, "closed", "2026-08-24T15:00:00Z")})
	})
	c, err := New("", f.server.URL+"/", 100)
	if err != nil {
		t.Fatal(err)
	}
	q, err := config.Default().ResolvePRs(config.PRRequest{Author: "octocat"}, prClock)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.CollectPRs(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 1 || len(got.PRs[0].Matches) != 2 || !got.PRs[0].MatchesComplete || got.Cost.HistoryRequests != 1 || got.Truncated {
		t.Fatalf("history: %+v", got)
	}
	if got.PRs[0].Matches[0].ID == got.PRs[0].Matches[1].ID || *got.PRs[0].PR.State != model.PRStateOpen {
		t.Fatal("lost distinct occurrence/current state")
	}
}
func TestPRMergeConflict(t *testing.T) {
	f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search/issues" {
			p := prPayload(1)
			p["state"] = "closed"
			p["created_at"] = "2020-01-01T00:00:00Z"
			p["repository_url"] = "https://api.github.com/repos/acme/api"
			p["pull_request"] = map[string]any{"merged_at": "2026-08-24T15:00:00Z"}
			prJSON(t, w, map[string]any{"total_count": 1, "items": []any{p}})
			return
		}
		prJSON(t, w, []any{prEvent(1, "merged", "2026-08-24T16:00:00Z")})
	})
	c, err := New("", f.server.URL+"/", 100)
	if err != nil {
		t.Fatal(err)
	}
	q, err := config.Default().ResolvePRs(config.PRRequest{Author: "octocat"}, prClock)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.CollectPRs(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 0 || !got.Truncated || got.Coverage.EvidenceComplete {
		t.Fatalf("invented conflicting merge: %+v", got)
	}
}

func TestPRClosureClassification(t *testing.T) {
	at := prClock.Add(-time.Hour)
	later := at.Add(time.Minute)
	before := at.Add(-time.Minute)
	cases := []struct {
		name     string
		events   []prLifecycle
		complete bool
		want     int
		gap      bool
	}{
		{"merge closure", []prLifecycle{{1, "closed", at}, {2, "merged", at}}, true, 1, false},
		{"earlier cycle", []prLifecycle{{1, "closed", before}, {2, "reopened", at}, {3, "merged", later}, {4, "closed", later}}, true, 2, false},
		{"same second ambiguity", []prLifecycle{{1, "closed", at}, {2, "reopened", at}}, true, 0, true},
		{"ambiguity keeps independent closure", []prLifecycle{{1, "closed", before}, {2, "reopened", before}, {3, "closed", later}}, true, 1, true},
		{"incomplete history", []prLifecycle{{1, "closed", at}}, false, 0, true},
		{"unpairable closure", []prLifecycle{{1, "closed", before}, {2, "merged", at}}, true, 1, true},
		{"conflicting event identity", []prLifecycle{{1, "closed", at}, {1, "reopened", later}}, true, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := New("", "", 100)
			if err != nil {
				t.Fatal(err)
			}
			s, err := c.newPRSession(10)
			if err != nil {
				t.Fatal(err)
			}
			p := model.PullRequest{Repo: "acme/api", Number: 1}
			_, matches, _ := s.classifyHistory(p, tc.events, tc.complete)
			if len(matches) != tc.want || (!s.coverage.EvidenceComplete) != tc.gap {
				t.Fatalf("matches=%+v coverage=%+v", matches, s.coverage)
			}
		})
	}
}

func TestPRHistorySettlesClosedStateWithoutRecordTimestamp(t *testing.T) {
	f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search/issues" {
			p := prPayload(1)
			p["state"] = "closed"
			p["closed_at"] = nil
			p["repository_url"] = "https://api.github.com/repos/acme/api"
			p["pull_request"] = map[string]any{}
			prJSON(t, w, map[string]any{"total_count": 1, "items": []any{p}})
			return
		}
		prJSON(t, w, []any{prEvent(1, "closed", "2026-08-24T14:00:00Z")})
	})
	c, e := New("", f.server.URL+"/", 100)
	if e != nil {
		t.Fatal(e)
	}
	q, e := config.Default().ResolvePRs(config.PRRequest{Author: "octocat", State: "closed", TimeBasis: "created"}, prClock)
	if e != nil {
		t.Fatal(e)
	}
	got, e := c.CollectPRs(context.Background(), q)
	if e != nil {
		t.Fatal(e)
	}
	if got.Count != 1 || got.PRs[0].PR.State == nil || *got.PRs[0].PR.State != model.PRStateClosed {
		t.Fatalf("history did not resolve known closed disposition: %+v", got)
	}
}

func TestPRMissingCurrentStateDoesNotUseHistoricalClosureTime(t *testing.T) {
	f := newPRFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search/issues" {
			p := prPayload(1)
			delete(p, "state")
			p["closed_at"] = "2026-08-24T14:00:00Z"
			p["repository_url"] = "https://api.github.com/repos/acme/api"
			p["pull_request"] = map[string]any{}
			prJSON(t, w, map[string]any{"total_count": 1, "items": []any{p}})
			return
		}
		prJSON(t, w, []any{prEvent(1, "closed", "2026-08-24T14:00:00Z"), prEvent(2, "reopened", "2026-09-01T00:00:00Z")})
	})
	c, e := New("", f.server.URL+"/", 100)
	if e != nil {
		t.Fatal(e)
	}
	q, e := config.Default().ResolvePRs(config.PRRequest{Author: "octocat", State: "closed", TimeBasis: "created"}, prClock)
	if e != nil {
		t.Fatal(e)
	}
	got, e := c.CollectPRs(context.Background(), q)
	if e != nil {
		t.Fatal(e)
	}
	if got.Count != 0 || !got.Truncated {
		t.Fatal("missing current state was guessed from historical closure")
	}
}
