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
	if q.Provider != model.ProviderGitHub {
		t.Errorf("provider = %q, want default github", q.Provider)
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

func TestResolveExplicitProvider(t *testing.T) {
	q, err := Default().Resolve(Request{
		Provider: "gitlab",
		Author:   "x",
		Scope:    "repos",
		Repos:    []string{"skaphos/sting"},
	}, time.Now())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if q.Provider != model.ProviderGitLab {
		t.Errorf("provider = %q, want gitlab", q.Provider)
	}
}

func TestResolveInvalidProvider(t *testing.T) {
	if _, err := Default().Resolve(Request{Provider: "bogus", Author: "x"}, time.Now()); err == nil {
		t.Fatal("want error for invalid provider")
	}
}

func TestResolveGitLabSearchUnsupported(t *testing.T) {
	_, err := Default().Resolve(Request{
		Provider: "gitlab",
		Author:   "x",
		Scope:    "search",
	}, time.Now())
	if err == nil {
		t.Fatal("want error for gitlab search")
	}
}

// TestResolveRejectsGitHubIdentifierInjection covers the search-qualifier
// injection fix: for GitHub, an author/org/repo that could break out of a
// commit-search qualifier (embedded space or colon, or a smuggled qualifier)
// must be rejected before it ever reaches the client.
func TestResolveRejectsGitHubIdentifierInjection(t *testing.T) {
	authors := []string{
		"victim author:attacker", // smuggled second qualifier
		"John Doe",               // bare space corrupts the query
		"x org:secret-org",       // smuggled org qualifier
		`victim"quote`,           // quote could break quoting
	}
	for _, author := range authors {
		if _, err := Default().Resolve(Request{Author: author}, time.Now()); err == nil {
			t.Errorf("Resolve(author=%q): want rejection, got nil error", author)
		}
	}

	if _, err := Default().Resolve(Request{
		Author: "x", Scope: "org", Org: "secret org:evil",
	}, time.Now()); err == nil {
		t.Error("Resolve(org with space/colon): want rejection")
	}

	if _, err := Default().Resolve(Request{
		Author: "x", Scope: "repos", Repos: []string{"owner/repo bad:x"},
	}, time.Now()); err == nil {
		t.Error("Resolve(repo with space/colon): want rejection")
	}
}

// TestResolveAcceptsValidGitHubAuthors confirms the validation does not reject
// legitimate GitHub logins or emails, including the "Name <addr>" form.
func TestResolveAcceptsValidGitHubAuthors(t *testing.T) {
	authors := []string{
		"mfacenet",
		"octo-cat",
		"user@example.com",
		"Mended Link <mended@example.com>",
	}
	for _, author := range authors {
		if _, err := Default().Resolve(Request{Author: author}, time.Now()); err != nil {
			t.Errorf("Resolve(author=%q): want accepted, got %v", author, err)
		}
	}
}

// TestResolveGitLabSkipsGitHubValidation confirms GitLab values are not subject
// to the GitHub charset: GitLab authors are free text and its client
// URL-encodes them, so a spaced author must pass through unchanged.
func TestResolveGitLabSkipsGitHubValidation(t *testing.T) {
	q, err := Default().Resolve(Request{
		Provider: "gitlab",
		Author:   "John Doe",
		Scope:    "repos",
		Repos:    []string{"group/sub group/proj"},
	}, time.Now())
	if err != nil {
		t.Fatalf("Resolve(gitlab, spaced author/repo): %v", err)
	}
	if q.Author != "John Doe" {
		t.Errorf("Author = %q, want John Doe", q.Author)
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

func TestResolveDiffsImplyFiles(t *testing.T) {
	cfg := Default()
	yes := true
	max := 1234
	q, err := cfg.Resolve(Request{
		Author:       "x",
		IncludeDiffs: &yes,
		MaxDiffBytes: &max,
	}, time.Now())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !q.IncludeDiffs {
		t.Error("expected IncludeDiffs override to true")
	}
	if !q.IncludeFiles {
		t.Error("IncludeDiffs should imply IncludeFiles")
	}
	if q.MaxDiffBytes != 1234 {
		t.Errorf("MaxDiffBytes = %d, want 1234", q.MaxDiffBytes)
	}
}
