// SPDX-License-Identifier: MIT
package activity_test

import (
	"reflect"
	"testing"

	"github.com/skaphos/sting/internal/activity"
	"github.com/skaphos/sting/model"
)

func path(p string) model.ChangedPath { return model.ChangedPath{Path: p, Status: "modified"} }

func TestCorrelateRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		paths     []model.ChangedPath
		commits   []model.ActivityCommit
		wantBasis string // "" means the path must be left unattributed
		wantRule  string
		wantSHAs  []string
	}{
		{
			name:  "observed when per-commit file data lists the path",
			paths: []model.ChangedPath{path("internal/render/activity.go")},
			commits: []model.ActivityCommit{{
				SHA: "aaa", Message: "chore: unrelated summary", Enriched: true,
				Files: []model.File{{Path: "internal/render/activity.go"}},
			}},
			wantBasis: model.BasisObserved,
			wantSHAs:  []string{"aaa"},
		},
		{
			name:  "observed matches a rename's previous path",
			paths: []model.ChangedPath{path("old/name.go")},
			commits: []model.ActivityCommit{{
				SHA: "aaa", Message: "refactor: move", Enriched: true,
				Files: []model.File{{Path: "new/name.go", PreviousPath: "old/name.go"}},
			}},
			wantBasis: model.BasisObserved,
			wantSHAs:  []string{"aaa"},
		},
		{
			name:  "inferred path-mention when the body names the path verbatim",
			paths: []model.ChangedPath{path("model/activity.go")},
			commits: []model.ActivityCommit{{
				SHA: "bbb", Message: "feat: types\n\nAdds model/activity.go with the new types.",
			}},
			wantBasis: model.BasisInferred,
			wantRule:  model.RulePathMention,
			wantSHAs:  []string{"bbb"},
		},
		{
			name:  "inferred scope-match when a conventional scope matches a leading segment",
			paths: []model.ChangedPath{path("render/view.go")},
			commits: []model.ActivityCommit{{
				SHA: "ccc", Message: "feat(render): add the activity view",
			}},
			wantBasis: model.BasisInferred,
			wantRule:  model.RuleScopeMatch,
			wantSHAs:  []string{"ccc"},
		},
		{
			name:  "breaking-change scope still matches",
			paths: []model.ChangedPath{path("render/view.go")},
			commits: []model.ActivityCommit{{
				SHA: "ccc", Message: "feat(render)!: rewrite the activity view",
			}},
			wantBasis: model.BasisInferred,
			wantRule:  model.RuleScopeMatch,
			wantSHAs:  []string{"ccc"},
		},
		{
			name:  "no rule matches leaves the path unattributed",
			paths: []model.ChangedPath{path("unrelated/file.go")},
			commits: []model.ActivityCommit{{
				SHA: "ddd", Message: "chore: tidy up",
			}},
			wantBasis: "",
		},
		{
			name:  "a parenthetical that is not a conventional scope does not match",
			paths: []model.ChangedPath{path("render/view.go")},
			commits: []model.ActivityCommit{{
				SHA: "eee", Message: "tidy the render (render) helpers",
			}},
			wantBasis: "",
		},
		{
			name:  "a top-level file cannot match the scope rule",
			paths: []model.ChangedPath{path("README.md")},
			commits: []model.ActivityCommit{{
				SHA: "fff", Message: "docs(README.md): update",
			}},
			// There is no leading directory segment, so the scope rule cannot
			// fire; the path-mention rule does, because the summary names it.
			wantBasis: model.BasisInferred,
			wantRule:  model.RulePathMention,
			wantSHAs:  []string{"fff"},
		},
		{
			name:  "an unenriched commit can never be observed",
			paths: []model.ChangedPath{path("a/b.go")},
			commits: []model.ActivityCommit{{
				SHA: "ggg", Message: "chore: unrelated", Enriched: false,
				Files: []model.File{{Path: "a/b.go"}}, // present but not fetched
			}},
			wantBasis: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := activity.Correlate(tt.paths, tt.commits)

			if tt.wantBasis == "" {
				if len(got) != 0 {
					t.Fatalf("expected no attribution, got %+v", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("got %d correlations, want 1: %+v", len(got), got)
			}
			c := got[0]
			if c.Basis != tt.wantBasis {
				t.Errorf("Basis = %q, want %q", c.Basis, tt.wantBasis)
			}
			if c.Rule != tt.wantRule {
				t.Errorf("Rule = %q, want %q", c.Rule, tt.wantRule)
			}
			if !reflect.DeepEqual(c.SHAs, tt.wantSHAs) {
				t.Errorf("SHAs = %v, want %v", c.SHAs, tt.wantSHAs)
			}
		})
	}
}

// TestCorrelateFirstMatchWins pins the rule ordering. Observation must beat
// inference whenever both are available — presenting the weaker basis when the
// stronger one exists would understate what is actually known.
func TestCorrelateFirstMatchWins(t *testing.T) {
	t.Parallel()

	commits := []model.ActivityCommit{
		{ // would match path-mention
			SHA: "mention", Message: "feat: touches render/view.go",
		},
		{ // matches observed
			SHA: "observed", Message: "chore: nothing relevant", Enriched: true,
			Files: []model.File{{Path: "render/view.go"}},
		},
		{ // would match scope-match
			SHA: "scope", Message: "feat(render): something",
		},
	}

	got := activity.Correlate([]model.ChangedPath{path("render/view.go")}, commits)
	if len(got) != 1 {
		t.Fatalf("got %d correlations, want 1", len(got))
	}
	if got[0].Basis != model.BasisObserved {
		t.Errorf("Basis = %q, want observed to win over both inference rules", got[0].Basis)
	}
	if !reflect.DeepEqual(got[0].SHAs, []string{"observed"}) {
		t.Errorf("SHAs = %v, want only the observed commit", got[0].SHAs)
	}
}

// TestCorrelatePathMentionBeatsScopeMatch: path-mention is the stronger of the
// two inference rules and must be preferred.
func TestCorrelatePathMentionBeatsScopeMatch(t *testing.T) {
	t.Parallel()
	commits := []model.ActivityCommit{
		{SHA: "scope", Message: "feat(render): something"},
		{SHA: "mention", Message: "fix: correct render/view.go"},
	}
	got := activity.Correlate([]model.ChangedPath{path("render/view.go")}, commits)
	if len(got) != 1 {
		t.Fatalf("got %d correlations, want 1", len(got))
	}
	if got[0].Rule != model.RulePathMention {
		t.Errorf("Rule = %q, want path-mention to win over scope-match", got[0].Rule)
	}
}

// TestCorrelateDeterministic is FR-023: byte-identical output across repeated
// runs. Any dependence on map iteration order would show up here.
func TestCorrelateDeterministic(t *testing.T) {
	t.Parallel()

	paths := []model.ChangedPath{
		path("z/last.go"), path("a/first.go"), path("m/middle.go"), path("render/view.go"),
	}
	commits := []model.ActivityCommit{
		{SHA: "ccc", Message: "feat(render): view"},
		{SHA: "aaa", Message: "feat: a/first.go and m/middle.go"},
		{SHA: "bbb", Message: "chore: z/last.go", Enriched: true,
			Files: []model.File{{Path: "z/last.go"}}},
	}

	first := activity.Correlate(paths, commits)
	for i := range 20 {
		got := activity.Correlate(paths, commits)
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d differs:\n got: %+v\nwant: %+v", i, got, first)
		}
	}

	// Results are path-sorted so the output order does not depend on the input
	// order of the change set.
	for i := 1; i < len(first); i++ {
		if first[i-1].Path > first[i].Path {
			t.Errorf("correlations are not path-sorted: %q before %q", first[i-1].Path, first[i].Path)
		}
	}
}

// TestCorrelateMultipleSHAsSorted keeps a multi-commit attribution stable.
func TestCorrelateMultipleSHAsSorted(t *testing.T) {
	t.Parallel()
	commits := []model.ActivityCommit{
		{SHA: "zzz", Message: "feat: a/b.go", Enriched: true, Files: []model.File{{Path: "a/b.go"}}},
		{SHA: "aaa", Message: "fix: a/b.go", Enriched: true, Files: []model.File{{Path: "a/b.go"}}},
		{SHA: "mmm", Message: "fix: a/b.go", Enriched: true, Files: []model.File{{Path: "a/b.go"}}},
	}
	got := activity.Correlate([]model.ChangedPath{path("a/b.go")}, commits)
	if len(got) != 1 {
		t.Fatalf("got %d correlations, want 1", len(got))
	}
	want := []string{"aaa", "mmm", "zzz"}
	if !reflect.DeepEqual(got[0].SHAs, want) {
		t.Errorf("SHAs = %v, want %v (sorted)", got[0].SHAs, want)
	}
}

func TestCorrelateEmptyInputs(t *testing.T) {
	t.Parallel()
	if got := activity.Correlate(nil, nil); got != nil {
		t.Errorf("Correlate(nil, nil) = %v, want nil", got)
	}
	if got := activity.Correlate([]model.ChangedPath{path("a.go")}, nil); got != nil {
		t.Errorf("Correlate with no commits = %v, want nil", got)
	}
	if got := activity.Correlate(nil, []model.ActivityCommit{{SHA: "a"}}); got != nil {
		t.Errorf("Correlate with no paths = %v, want nil", got)
	}
}

// TestCorrelateNeverClaimsObservedWithoutEnrichment is the invariant that keeps
// the whole attribution story honest: a correlation may carry "observed" only
// when every SHA it names was genuinely enriched.
//
// This is the test that stops inference being presented as observation.
func TestCorrelateNeverClaimsObservedWithoutEnrichment(t *testing.T) {
	t.Parallel()

	paths := []model.ChangedPath{
		path("a/one.go"), path("b/two.go"), path("c/three.go"), path("render/view.go"),
	}
	commits := []model.ActivityCommit{
		{SHA: "enriched1", Message: "feat: work", Enriched: true,
			Files: []model.File{{Path: "a/one.go"}}},
		{SHA: "plain1", Message: "feat: touches b/two.go"},
		{SHA: "plain2", Message: "feat(render): view work",
			Files: []model.File{{Path: "c/three.go"}}}, // files present but NOT enriched
	}

	enriched := map[string]bool{}
	for _, c := range commits {
		if c.Enriched {
			enriched[c.SHA] = true
		}
	}

	for _, c := range activity.Correlate(paths, commits) {
		if c.Basis != model.BasisObserved {
			continue
		}
		for _, sha := range c.SHAs {
			if !enriched[sha] {
				t.Errorf("correlation for %q claims observed provenance for %q, "+
					"which was never enriched — inference is being presented as observation",
					c.Path, sha)
			}
		}
	}
}
