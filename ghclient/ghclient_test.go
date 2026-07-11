// SPDX-License-Identifier: MIT
package ghclient

import (
	"testing"
	"time"

	"github.com/skaphos/sting/model"
)

func TestBuildSearchQuery(t *testing.T) {
	since := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)

	got := buildSearchQuery(model.Query{Author: "mfacenet", Since: since, Until: until})
	want := "author:mfacenet author-date:2026-05-22T00:00:00Z..2026-05-29T00:00:00Z"
	if got != want {
		t.Errorf("buildSearchQuery = %q, want %q", got, want)
	}
}

func TestBuildSearchQueryOpenEnded(t *testing.T) {
	got := buildSearchQuery(model.Query{Author: "x"})
	if got != "author:x" {
		t.Errorf("buildSearchQuery = %q, want %q", got, "author:x")
	}
}

func TestBuildSearchQueryEmail(t *testing.T) {
	got := buildSearchQuery(model.Query{Author: "shawn.stratton@alaskaair.com"})
	want := "author-email:shawn.stratton@alaskaair.com"
	if got != want {
		t.Errorf("buildSearchQuery = %q, want %q", got, want)
	}
}

func TestBuildSearchQueryEmailWithOrg(t *testing.T) {
	got := buildSearchQuery(model.Query{Author: "shawn.stratton@alaskaair.com", Org: "Alaska-Airlines-Shared"})
	want := "author-email:shawn.stratton@alaskaair.com org:Alaska-Airlines-Shared"
	if got != want {
		t.Errorf("buildSearchQuery = %q, want %q", got, want)
	}
}

func TestAuthorQualifier(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"mfacenet", "author:mfacenet"},
		{"shawn.stratton@alaskaair.com", "author-email:shawn.stratton@alaskaair.com"},
		{"octocat", "author:octocat"},
		{"user@example.com", "author-email:user@example.com"},
		{"Mended Link <mended@example.com>", "author-email:mended@example.com"},
	}
	for _, tt := range tests {
		if got := authorQualifier(tt.in); got != tt.want {
			t.Errorf("authorQualifier(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAuthorMatches(t *testing.T) {
	cm := model.Commit{Author: "octocat", Email: "octo@example.com"}
	cases := []struct {
		author string
		want   bool
	}{
		{"octocat", true},
		{"OCTOCAT", true},
		{"octo@example.com", true},
		{"Octo Cat <octo@example.com>", true}, // angle-bracket form normalizes to the bare email
		{"someoneelse", false},
		{"other@example.com", false},
		{"", false},
	}
	for _, c := range cases {
		if got := authorMatches(cm, c.author); got != c.want {
			t.Errorf("authorMatches(%q) = %v, want %v", c.author, got, c.want)
		}
	}
}

// TestSearchQualifierValueEscaping covers the defense-in-depth quoting in the
// client: safe identifier values pass through verbatim, while anything that
// could break out of a qualifier is double-quoted with quotes and backslashes
// escaped.
func TestSearchQualifierValueEscaping(t *testing.T) {
	tests := []struct{ in, want string }{
		{"octocat", "octocat"},
		{"user@example.com", "user@example.com"},
		{"skaphos/sting", "skaphos/sting"},
		{"user+tag@example.com", "user+tag@example.com"},
		{"victim author:attacker", `"victim author:attacker"`},
		{"John Doe", `"John Doe"`},
		{`quote"inside`, `"quote\"inside"`},
		{`back\slash`, `"back\\slash"`},
	}
	for _, tt := range tests {
		if got := searchQualifierValue(tt.in); got != tt.want {
			t.Errorf("searchQualifierValue(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestBuildSearchQueryQuotesInjection verifies that even if an unsafe value
// reaches buildSearchQuery (config.Resolve would normally reject it first), it
// is quoted so it cannot inject a second qualifier.
func TestBuildSearchQueryQuotesInjection(t *testing.T) {
	got := buildSearchQuery(model.Query{Author: "victim author:attacker"})
	want := `author:"victim author:attacker"`
	if got != want {
		t.Errorf("buildSearchQuery = %q, want %q", got, want)
	}
}

func TestBuildSearchQueryWithOrg(t *testing.T) {
	got := buildSearchQuery(model.Query{Author: "mfacenet", Org: "Alaska-Airlines-Shared"})
	want := "author:mfacenet org:Alaska-Airlines-Shared"
	if got != want {
		t.Errorf("buildSearchQuery = %q, want %q", got, want)
	}
}

func TestBuildSearchQueryWithRepos(t *testing.T) {
	got := buildSearchQuery(model.Query{Author: "x", Repos: []string{"skaphos/sting", " skaphos/other "}})
	want := "author:x repo:skaphos/sting repo:skaphos/other"
	if got != want {
		t.Errorf("buildSearchQuery = %q, want %q", got, want)
	}
}

func TestSplitRepo(t *testing.T) {
	tests := []struct {
		in          string
		owner, repo string
		ok          bool
	}{
		{"skaphos/sting", "skaphos", "sting", true},
		{" skaphos/sting ", "skaphos", "sting", true},
		{"noslash", "", "", false},
		{"/missing", "", "", false},
		{"missing/", "", "", false},
	}
	for _, tt := range tests {
		owner, repo, ok := splitRepo(tt.in)
		if ok != tt.ok || owner != tt.owner || repo != tt.repo {
			t.Errorf("splitRepo(%q) = (%q,%q,%v), want (%q,%q,%v)",
				tt.in, owner, repo, ok, tt.owner, tt.repo, tt.ok)
		}
	}
}
