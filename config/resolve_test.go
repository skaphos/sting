// SPDX-License-Identifier: MIT
package config

import (
	"testing"
	"time"

	"github.com/skaphos/sting/model"
)

func TestResolveDefaults(t *testing.T) {
	cfg := Default()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)

	q, err := cfg.Resolve(Request{Author: "mfacenet"}, now)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if q.Author != "mfacenet" {
		t.Errorf("author = %q", q.Author)
	}
	if q.Scope != model.ScopeSearch {
		t.Errorf("scope = %q, want default search", q.Scope)
	}
	if !q.Until.Equal(now) {
		t.Errorf("until = %v, want now %v", q.Until, now)
	}
	wantSince := now.Add(-7 * 24 * time.Hour)
	if !q.Since.Equal(wantSince) {
		t.Errorf("since = %v, want %v (7d window)", q.Since, wantSince)
	}
}

func TestResolveRequiresAuthor(t *testing.T) {
	if _, err := Default().Resolve(Request{}, time.Now()); err == nil {
		t.Fatal("want error for missing author")
	}
}

func TestResolveExplicitWindow(t *testing.T) {
	now := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)
	q, err := Default().Resolve(Request{Author: "x", Window: "2w"}, now)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := now.Add(-14 * 24 * time.Hour); !q.Since.Equal(want) {
		t.Errorf("since = %v, want %v", q.Since, want)
	}
}

func TestResolveExplicitSinceUntil(t *testing.T) {
	q, err := Default().Resolve(Request{
		Author: "x",
		Since:  "2026-05-01",
		Until:  "2026-05-15",
	}, time.Now())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if q.Since.Format("2006-01-02") != "2026-05-01" {
		t.Errorf("since = %v", q.Since)
	}
	if q.Until.Format("2006-01-02") != "2026-05-15" {
		t.Errorf("until = %v", q.Until)
	}
}

func TestResolveSinceAfterUntil(t *testing.T) {
	_, err := Default().Resolve(Request{
		Author: "x",
		Since:  "2026-05-15",
		Until:  "2026-05-01",
	}, time.Now())
	if err == nil {
		t.Fatal("want error when since is after until")
	}
}

func TestResolveInvalidScope(t *testing.T) {
	if _, err := Default().Resolve(Request{Author: "x", Scope: "bogus"}, time.Now()); err == nil {
		t.Fatal("want error for invalid scope")
	}
}

func TestResolveStatsOverride(t *testing.T) {
	cfg := Default() // IncludeStats false
	yes := true
	q, err := cfg.Resolve(Request{Author: "x", IncludeStats: &yes}, time.Now())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !q.IncludeStats {
		t.Error("expected IncludeStats override to true")
	}
}
