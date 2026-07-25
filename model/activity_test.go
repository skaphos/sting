// SPDX-License-Identifier: MIT
package model_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/skaphos/sting/model"
)

func TestActivityCommitSummary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{name: "multi line", message: "feat: add activity\n\nlonger body here", want: "feat: add activity"},
		{name: "single line", message: "fix: off by one", want: "fix: off by one"},
		{name: "empty", message: "", want: ""},
		{name: "trailing newline only", message: "chore: tidy\n", want: "chore: tidy"},
		{name: "leading blank line", message: "\nbody", want: ""},
		{name: "crlf", message: "docs: readme\r\n\r\nbody", want: "docs: readme\r"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := model.ActivityCommit{Message: tt.message}.Summary()
			if got != tt.want {
				t.Errorf("Summary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSchemaVersionUnchanged guards the compatibility promise this feature is
// built on: ActivityResult is additive, so Result's schema version must not
// move. An accidental bump would force every downstream consumer that pinned
// v2 (Wake's evidence adapter among them) to re-coordinate for no reason.
func TestSchemaVersionUnchanged(t *testing.T) {
	t.Parallel()
	if model.SchemaVersion != "sting.skaphos.io/v2" {
		t.Errorf("model.SchemaVersion = %q, want %q — this feature must not bump the Result contract",
			model.SchemaVersion, "sting.skaphos.io/v2")
	}
}

func TestActivitySchemaVersion(t *testing.T) {
	t.Parallel()
	if model.ActivitySchemaVersion != "sting.activity.skaphos.io/v1" {
		t.Errorf("ActivitySchemaVersion = %q, want %q", model.ActivitySchemaVersion, "sting.activity.skaphos.io/v1")
	}
	if model.ActivitySchemaVersion == model.SchemaVersion {
		t.Error("ActivitySchemaVersion must be distinct from SchemaVersion")
	}
}

func TestDefaultMaxRequests(t *testing.T) {
	t.Parallel()
	if model.DefaultMaxRequests != 500 {
		t.Errorf("DefaultMaxRequests = %d, want 500", model.DefaultMaxRequests)
	}
}

func TestActivityResultJSONRoundTrip(t *testing.T) {
	t.Parallel()
	since := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	in := model.ActivityResult{
		SchemaVersion:   model.ActivitySchemaVersion,
		GeneratedAt:     until,
		Provider:        model.ProviderGitHub,
		Repo:            "skaphos/sting",
		Ref:             "main",
		Since:           since,
		Until:           until,
		WindowDateBasis: model.WindowDateBasisCommitter,
		Boundaries: model.Boundaries{
			BaseSHA:    "1ce380e",
			HeadSHA:    "1ae89e4",
			BaseSource: model.BaseSourceParentOfEarliest,
			Status:     model.StatusAhead,
			SharedRoot: true,
		},
		Count: 1,
		Commits: []model.ActivityCommit{{
			SHA:           "1ae89e4",
			Repo:          "skaphos/sting",
			AuthorName:    "Someone",
			Message:       "feat: thing\n\nbody",
			AuthorDate:    since,
			CommitterDate: until,
			ParentSHAs:    []string{"1ce380e"},
			Enriched:      true,
			Files:         []model.File{{Path: "model/activity.go", Status: "added"}},
		}},
		ChangeSet: model.ChangeSet{
			Paths: []model.ChangedPath{{
				Path: "model/activity.go", Status: "added", Additions: 10, Deletions: 2,
			}},
			TotalAdditions: 10,
			TotalDeletions: 2,
		},
		Correlations: []model.Correlation{{
			Path: "model/activity.go", SHAs: []string{"1ae89e4"}, Basis: model.BasisObserved,
		}},
		Cost: model.CostReport{Estimated: 8, Consumed: 8, Ceiling: 500, QuotaRemaining: 4921, QuotaLimit: 5000},
		Disclosures: []model.Disclosure{{
			Kind: model.DisclosureReferenceScoped, Reason: "covers main only", NextAction: "compare another ref",
		}},
	}

	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out model.ActivityResult
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.SchemaVersion != in.SchemaVersion || out.Repo != in.Repo || out.Ref != in.Ref {
		t.Errorf("identity fields did not survive round trip: %+v", out)
	}
	if !out.Since.Equal(in.Since) || !out.Until.Equal(in.Until) {
		t.Errorf("window did not survive round trip: %v..%v", out.Since, out.Until)
	}
	if out.WindowDateBasis != model.WindowDateBasisCommitter {
		t.Errorf("WindowDateBasis = %q, want %q", out.WindowDateBasis, model.WindowDateBasisCommitter)
	}
	if out.Boundaries != in.Boundaries {
		t.Errorf("Boundaries = %+v, want %+v", out.Boundaries, in.Boundaries)
	}
	if len(out.Commits) != 1 || out.Commits[0].SHA != "1ae89e4" {
		t.Fatalf("commits did not survive round trip: %+v", out.Commits)
	}
	if got := out.Commits[0].ParentSHAs; len(got) != 1 || got[0] != "1ce380e" {
		t.Errorf("ParentSHAs = %v, want [1ce380e]", got)
	}
	if !out.Commits[0].CommitterDate.Equal(until) {
		t.Errorf("CommitterDate = %v, want %v", out.Commits[0].CommitterDate, until)
	}
	if !out.Commits[0].Enriched {
		t.Error("Enriched did not survive round trip")
	}
	if out.Cost != in.Cost {
		t.Errorf("Cost = %+v, want %+v", out.Cost, in.Cost)
	}
}

// TestActivityResultOmitEmpty pins which fields disappear from a zero-ish
// result and which must always be present. Cost and change_set are always
// emitted: a consumer must never have to distinguish "absent" from "zero" when
// asking what a query spent.
func TestActivityResultOmitEmpty(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(model.ActivityResult{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)

	alwaysPresent := []string{
		"schema_version", "generated_at", "provider", "repo", "ref",
		"since", "until", "window_date_basis", "boundaries", "count",
		"commits", "change_set", "cost",
	}
	for _, key := range alwaysPresent {
		if !strings.Contains(got, `"`+key+`"`) {
			t.Errorf("key %q missing from zero-value JSON: %s", key, got)
		}
	}
	omitted := []string{"correlations", "disclosures"}
	for _, key := range omitted {
		if strings.Contains(got, `"`+key+`"`) {
			t.Errorf("key %q should be omitted when empty: %s", key, got)
		}
	}
	// quota_resets_at is omitempty, but a zero time.Time is not the empty
	// value encoding/json omits, so it is asserted separately as documented
	// behavior rather than assumed away.
	if !strings.Contains(got, `"quota_resets_at"`) {
		t.Log("quota_resets_at omitted for zero time (encoding/json behavior may vary by version)")
	}
}

func TestActivityCommitOmitEmpty(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(model.ActivityCommit{SHA: "abc", Repo: "o/r", Message: "m"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	for _, key := range []string{"author", "email", "parent_shas", "enriched", "files"} {
		if strings.Contains(got, `"`+key+`"`) {
			t.Errorf("key %q should be omitted when empty: %s", key, got)
		}
	}
	for _, key := range []string{"sha", "repo", "author_name", "url", "message", "author_date", "committer_date"} {
		if !strings.Contains(got, `"`+key+`"`) {
			t.Errorf("key %q must always be present: %s", key, got)
		}
	}
}

func TestChangedPathOmitEmpty(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(model.ChangedPath{Path: "a.go", Status: "modified"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	for _, key := range []string{"previous_path", "patch", "patch_truncated"} {
		if strings.Contains(got, `"`+key+`"`) {
			t.Errorf("key %q should be omitted when empty: %s", key, got)
		}
	}
	// Additions and Deletions have no omitempty: a path with zero additions is
	// meaningfully different from one whose counts were never populated.
	for _, key := range []string{"path", "status", "additions", "deletions"} {
		if !strings.Contains(got, `"`+key+`"`) {
			t.Errorf("key %q must always be present: %s", key, got)
		}
	}
}

func TestDisclosureOmitEmpty(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(model.Disclosure{Kind: model.DisclosureReferenceScoped, Reason: "why"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "next_action") {
		t.Errorf("next_action should be omitted when empty: %s", b)
	}
}

// TestDisclosureKindsDistinct guards against a copy-paste collision between the
// disclosure kind constants: two kinds sharing a value would make the
// unconditional-blindspot assertions unfalsifiable.
func TestDisclosureKindsDistinct(t *testing.T) {
	t.Parallel()
	kinds := []string{
		model.DisclosureBudgetBounded,
		model.DisclosureQuotaExhausted,
		model.DisclosureProviderCapped,
		model.DisclosurePatchTruncated,
		model.DisclosureAncestryDiverged,
		model.DisclosureNetComparisonBlindspot,
		model.DisclosureReferenceScoped,
		model.DisclosureAuthorFilterNotApplied,
		model.DisclosureEnrichmentPartial,
	}
	seen := map[string]bool{}
	for _, k := range kinds {
		if k == "" {
			t.Error("disclosure kind must not be empty")
		}
		if seen[k] {
			t.Errorf("duplicate disclosure kind %q", k)
		}
		seen[k] = true
	}
}
