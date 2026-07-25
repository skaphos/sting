// SPDX-License-Identifier: MIT
package render_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/skaphos/sting/internal/render"
	"github.com/skaphos/sting/model"
)

func sampleActivity() model.ActivityResult {
	since := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	return model.ActivityResult{
		SchemaVersion:   model.ActivitySchemaVersion,
		GeneratedAt:     until,
		Provider:        model.ProviderGitHub,
		Repo:            "skaphos/sting",
		Ref:             "main",
		Since:           since,
		Until:           until,
		WindowDateBasis: model.WindowDateBasisCommitter,
		Boundaries: model.Boundaries{
			BaseSHA:    "1ce380ecafe0000",
			HeadSHA:    "1ae89e4beef1111",
			BaseSource: model.BaseSourceParentOfEarliest,
			Status:     model.StatusAhead,
			SharedRoot: true,
		},
		Count: 2,
		Commits: []model.ActivityCommit{
			{
				SHA: "1ae89e4beef1111", Repo: "skaphos/sting", AuthorName: "Octo Cat",
				Message:       "feat(render): add activity view\n\nRenders disclosures visibly.",
				CommitterDate: until,
			},
			{
				SHA: "aaa2222", Repo: "skaphos/sting", AuthorName: "Octo Cat",
				Message:       "fix: boundary off-by-one",
				CommitterDate: since,
			},
		},
		ChangeSet: model.ChangeSet{
			Paths: []model.ChangedPath{
				{Path: "internal/render/activity.go", Status: "added", Additions: 120, Deletions: 0},
				{Path: "model/activity.go", Status: "modified", Additions: 8, Deletions: 3},
			},
			TotalAdditions: 128,
			TotalDeletions: 3,
		},
		Cost: model.CostReport{
			Consumed: 8, Ceiling: 500, QuotaRemaining: 4921, QuotaLimit: 5000,
			QuotaResetsAt: time.Date(2026, 7, 25, 14, 32, 0, 0, time.UTC),
		},
		Disclosures: []model.Disclosure{
			{Kind: model.DisclosureReferenceScoped, Reason: "This covers main only.", NextAction: "Use --ref."},
			{Kind: model.DisclosureNetComparisonBlindspot, Reason: "Start and end states are compared."},
		},
	}
}

func TestRenderActivityMarkdownIncludesEverything(t *testing.T) {
	t.Parallel()
	out, err := render.RenderActivity(sampleActivity(), render.FormatMarkdown)
	if err != nil {
		t.Fatalf("RenderActivity: %v", err)
	}

	// The resolved query and both boundary SHAs must be present, or the result
	// cannot be re-derived from what the reader sees.
	wants := []string{
		"# Activity:", "skaphos/sting",
		"main",               // resolved reference
		"2026-07-18",         // window start
		"2026-07-25",         // window end
		"committer",          // which date bounded the window
		"1ce380e",            // base SHA (shortened)
		"1ae89e4",            // head SHA (shortened)
		"parent-of-earliest", // how the base was chosen
		"## Commits (2)",
		"add activity view",
		"boundary off-by-one",
		"## Change set",
		"internal/render/activity.go",
		"model/activity.go",
		"+128",
		"## Cost",
		"8 consumed",
		"500",
		"## Disclosures",
	}
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("Markdown missing %q\n---\n%s", want, out)
		}
	}
}

// TestRenderActivityDisclosuresAreVisible: an agent that misses the blind-spot
// disclosure will overstate the evidence, so disclosures render as their own
// section rather than a footnote.
func TestRenderActivityDisclosuresAreVisible(t *testing.T) {
	t.Parallel()
	out, err := render.RenderActivity(sampleActivity(), render.FormatMarkdown)
	if err != nil {
		t.Fatalf("RenderActivity: %v", err)
	}
	if !strings.Contains(out, "## Disclosures") {
		t.Fatal("disclosures are not rendered as a visible section")
	}
	for _, kind := range []string{model.DisclosureReferenceScoped, model.DisclosureNetComparisonBlindspot} {
		if !strings.Contains(out, kind) {
			t.Errorf("disclosure kind %q not rendered", kind)
		}
	}
	if !strings.Contains(out, "Next:") {
		t.Error("a disclosure's next action is not rendered")
	}
}

// TestRenderActivityJSONIsTheContract: the JSON output must be the result
// verbatim, since it is what downstream consumers pin.
func TestRenderActivityJSONIsTheContract(t *testing.T) {
	t.Parallel()
	in := sampleActivity()
	out, err := render.RenderActivity(in, render.FormatJSON)
	if err != nil {
		t.Fatalf("RenderActivity: %v", err)
	}

	var got model.ActivityResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	want, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	gotRe, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if string(want) != string(gotRe) {
		t.Errorf("JSON is not the result verbatim:\n got: %s\nwant: %s", gotRe, want)
	}
}

// TestRenderActivityPathsRenderInSortedOrder guards the determinism promise at
// the rendering layer.
func TestRenderActivityPathsRenderInSortedOrder(t *testing.T) {
	t.Parallel()
	r := sampleActivity()
	r.ChangeSet.Paths = []model.ChangedPath{
		{Path: "z.go", Status: "modified"},
		{Path: "a.go", Status: "modified"},
		{Path: "m.go", Status: "modified"},
	}
	// The renderer presents whatever order it is given; sorting happens in the
	// client. This asserts the rendering preserves that order faithfully rather
	// than reshuffling it (e.g. via map iteration).
	out, err := render.RenderActivity(r, render.FormatMarkdown)
	if err != nil {
		t.Fatalf("RenderActivity: %v", err)
	}
	zi, ai, mi := strings.Index(out, "z.go"), strings.Index(out, "a.go"), strings.Index(out, "m.go")
	if zi >= ai || ai >= mi {
		t.Errorf("renderer reordered paths: z=%d a=%d m=%d", zi, ai, mi)
	}
}

func TestRenderActivityEmptyWindow(t *testing.T) {
	t.Parallel()
	r := sampleActivity()
	r.Count = 0
	r.Commits = nil
	r.ChangeSet = model.ChangeSet{}
	r.Boundaries.Status = model.StatusIdentical

	out, err := render.RenderActivity(r, render.FormatMarkdown)
	if err != nil {
		t.Fatalf("RenderActivity: %v", err)
	}
	if !strings.Contains(out, "## Commits (0)") {
		t.Errorf("empty window not rendered as zero commits:\n%s", out)
	}
	if !strings.Contains(out, "No commits in this window") {
		t.Errorf("empty window has no explanatory line:\n%s", out)
	}
	// Cost is reported even for an empty result.
	if !strings.Contains(out, "## Cost") {
		t.Error("cost section missing from an empty result")
	}
}

func TestRenderActivityDivergedExplains(t *testing.T) {
	t.Parallel()
	r := sampleActivity()
	r.ChangeSet = model.ChangeSet{}
	r.Boundaries.Status = model.StatusDiverged
	r.Boundaries.SharedRoot = false

	out, err := render.RenderActivity(r, render.FormatMarkdown)
	if err != nil {
		t.Fatalf("RenderActivity: %v", err)
	}
	if !strings.Contains(out, "do not share ancestry") {
		t.Errorf("diverged result does not explain the empty change set:\n%s", out)
	}
}

func TestRenderActivityCorrelationsShowBasis(t *testing.T) {
	t.Parallel()
	r := sampleActivity()
	r.Correlations = []model.Correlation{
		{Path: "a.go", SHAs: []string{"1ae89e4beef1111"}, Basis: model.BasisObserved},
		{Path: "b.go", SHAs: []string{"aaa2222"}, Basis: model.BasisInferred, Rule: model.RulePathMention},
	}
	out, err := render.RenderActivity(r, render.FormatMarkdown)
	if err != nil {
		t.Fatalf("RenderActivity: %v", err)
	}
	// A reader must be able to tell observation from inference at a glance.
	if !strings.Contains(out, model.BasisObserved) || !strings.Contains(out, model.BasisInferred) {
		t.Errorf("correlation bases not rendered:\n%s", out)
	}
	if !strings.Contains(out, model.RulePathMention) {
		t.Errorf("inferred correlation does not name its rule:\n%s", out)
	}
}

func TestRenderActivityTruncatedChangeSetFlagged(t *testing.T) {
	t.Parallel()
	r := sampleActivity()
	r.ChangeSet.Truncated = true
	out, err := render.RenderActivity(r, render.FormatMarkdown)
	if err != nil {
		t.Fatalf("RenderActivity: %v", err)
	}
	if !strings.Contains(out, "truncated") {
		t.Errorf("a truncated change set is not flagged:\n%s", out)
	}
}

func TestRenderActivityUnknownFormat(t *testing.T) {
	t.Parallel()
	if _, err := render.RenderActivity(sampleActivity(), render.Format("xml")); err == nil {
		t.Error("expected an error for an unknown format")
	}
}

func TestActivityMarkdownNeverFails(t *testing.T) {
	t.Parallel()
	if got := render.ActivityMarkdown(model.ActivityResult{}); got == "" {
		t.Error("ActivityMarkdown returned empty output for a zero result")
	}
}

// TestRenderActivityPatchIsFenced mirrors the existing defense for commit
// patches: untrusted patch content must not be able to close the code block and
// render as live Markdown.
func TestRenderActivityPatchIsFenced(t *testing.T) {
	t.Parallel()
	r := sampleActivity()
	r.ChangeSet.Paths = []model.ChangedPath{{
		Path:   "evil.md",
		Status: "modified",
		Patch:  "@@ -1 +1 @@\n```\n# not a heading\n",
	}}
	out, err := render.RenderActivity(r, render.FormatMarkdown)
	if err != nil {
		t.Fatalf("RenderActivity: %v", err)
	}
	if !strings.Contains(out, "````") {
		t.Errorf("patch containing a fence was not wrapped in a longer fence:\n%s", out)
	}
}

func TestRenderActivityDefaultBranchLabel(t *testing.T) {
	t.Parallel()
	r := sampleActivity()
	r.Ref = ""
	out, err := render.RenderActivity(r, render.FormatMarkdown)
	if err != nil {
		t.Fatalf("RenderActivity: %v", err)
	}
	if !strings.Contains(out, "default branch") {
		t.Errorf("an unresolved reference is not labeled:\n%s", out)
	}
}
