package ghclient

import (
	"testing"
	"time"

	"github.com/skaphos/sting/internal/model"
)

func TestBuildSearchQuery(t *testing.T) {
	since := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)

	got := buildSearchQuery(model.Query{Author: "mfacenet", Since: since, Until: until})
	want := "author:mfacenet author-date:2026-05-22..2026-05-29"
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
