// SPDX-License-Identifier: MIT
package activity_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/skaphos/sting/internal/activity"
	"github.com/skaphos/sting/model"
)

func kinds(ds []model.Disclosure) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Kind)
	}
	return out
}

func has(ds []model.Disclosure, kind string) bool {
	for _, d := range ds {
		if d.Kind == kind {
			return true
		}
	}
	return false
}

// TestBuildUnconditionalDisclosures is FR-018: whenever a change set is
// presented, the two blind-spot statements accompany it — including on a
// completely clean result. If they became conditional, a reader would treat the
// ordinary case as more complete than it is.
func TestBuildUnconditionalDisclosures(t *testing.T) {
	t.Parallel()
	got := activity.Build(activity.DisclosureInput{Ref: "main", ChangeSetProduced: true})

	for _, want := range []string{model.DisclosureReferenceScoped, model.DisclosureNetComparisonBlindspot} {
		if !has(got, want) {
			t.Errorf("clean result missing unconditional disclosure %q; got %v", want, kinds(got))
		}
	}
	if len(got) != 2 {
		t.Errorf("a clean result produced %v, want exactly the two unconditional disclosures", kinds(got))
	}
}

// TestBuildNoChangeSetSkipsBlindSpots: with no comparison presented there are no
// comparison blind spots to disclose.
func TestBuildNoChangeSetSkipsBlindSpots(t *testing.T) {
	t.Parallel()
	got := activity.Build(activity.DisclosureInput{Ref: "main"})
	if has(got, model.DisclosureNetComparisonBlindspot) {
		t.Errorf("blind-spot disclosure emitted without a change set: %v", kinds(got))
	}
}

func TestBuildConditionalDisclosures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   activity.DisclosureInput
		want string
	}{
		{
			name: "diverged",
			in:   activity.DisclosureInput{Ref: "main", Diverged: true},
			want: model.DisclosureAncestryDiverged,
		},
		{
			name: "provider capped",
			in:   activity.DisclosureInput{Ref: "main", ChangeSetProduced: true, ProviderCapped: true},
			want: model.DisclosureProviderCapped,
		},
		{
			name: "patch truncated",
			in:   activity.DisclosureInput{Ref: "main", ChangeSetProduced: true, PatchTruncated: true},
			want: model.DisclosurePatchTruncated,
		},
		{
			name: "author filter",
			in:   activity.DisclosureInput{Ref: "main", ChangeSetProduced: true, AuthorFilter: "octocat"},
			want: model.DisclosureAuthorFilterNotApplied,
		},
		{
			name: "root commit base",
			in:   activity.DisclosureInput{Ref: "main", ChangeSetProduced: true, RootCommitBase: true},
			want: model.DisclosureNetComparisonBlindspot,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := activity.Build(tt.in)
			if !has(got, tt.want) {
				t.Errorf("missing %q; got %v", tt.want, kinds(got))
			}
		})
	}
}

// TestAuthorFilterNotDisclosedWithoutChangeSet: with no change set there is no
// asymmetry between the commit list and the comparison to disclose.
func TestAuthorFilterNotDisclosedWithoutChangeSet(t *testing.T) {
	t.Parallel()
	got := activity.Build(activity.DisclosureInput{Ref: "main", AuthorFilter: "octocat"})
	if has(got, model.DisclosureAuthorFilterNotApplied) {
		t.Errorf("author-filter disclosure emitted with no change set: %v", kinds(got))
	}
}

// TestBuildDeterministicOrder is FR-023 at the disclosure layer.
// TestBuildEmitsNoDuplicateKinds: two disclosures sharing a kind read as a
// rendering glitch and make the set harder to filter programmatically.
func TestBuildEmitsNoDuplicateKinds(t *testing.T) {
	t.Parallel()
	in := activity.DisclosureInput{
		Ref: "main", ChangeSetProduced: true, RootCommitBase: true,
		ProviderCapped: true, PatchTruncated: true, AuthorFilter: "octocat",
	}
	seen := map[string]int{}
	for _, d := range activity.Build(in) {
		seen[d.Kind]++
	}
	for kind, n := range seen {
		if n > 1 {
			t.Errorf("disclosure kind %q emitted %d times, want at most once", kind, n)
		}
	}
}

// TestRootCommitBaseStillCarriesTheGeneralBlindSpot: folding the root-commit
// caveat into the blind-spot disclosure must not drop the general warning.
func TestRootCommitBaseStillCarriesTheGeneralBlindSpot(t *testing.T) {
	t.Parallel()
	got := activity.Build(activity.DisclosureInput{
		Ref: "main", ChangeSetProduced: true, RootCommitBase: true,
	})
	var blindspot model.Disclosure
	for _, d := range got {
		if d.Kind == model.DisclosureNetComparisonBlindspot {
			blindspot = d
		}
	}
	if blindspot.Kind == "" {
		t.Fatalf("no blind-spot disclosure; got %v", kinds(got))
	}
	if !strings.Contains(blindspot.Reason, "root commit") {
		t.Errorf("root-commit caveat missing: %q", blindspot.Reason)
	}
	if !strings.Contains(blindspot.Reason, "start and end states") {
		t.Errorf("general net-comparison caveat was dropped: %q", blindspot.Reason)
	}
}

func TestBuildDeterministicOrder(t *testing.T) {
	t.Parallel()
	in := activity.DisclosureInput{
		Ref: "main", ChangeSetProduced: true, ProviderCapped: true,
		PatchTruncated: true, AuthorFilter: "octocat", RootCommitBase: true,
	}
	first := activity.Build(in)
	for i := range 10 {
		if got := activity.Build(in); !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d differs: %v vs %v", i, kinds(got), kinds(first))
		}
	}
}

// TestEveryDisclosureCarriesAReason is Constitution II: "bounded" with no stated
// reason is a defect, not a disclosure.
func TestEveryDisclosureCarriesAReason(t *testing.T) {
	t.Parallel()
	all := []model.Disclosure{
		activity.ReferenceScoped("main"),
		activity.ReferenceScoped(""),
		activity.NetComparisonBlindspot(),
		activity.AncestryDiverged(),
		activity.RepositoryRootBase(),
		activity.ProviderCapped(),
		activity.PatchTruncated(),
		activity.AuthorFilterNotApplied("octocat"),
		activity.BudgetBounded(12, 12),
		activity.QuotaExhausted("2026-07-25T14:32:00Z"),
		activity.QuotaExhausted(""),
		activity.EnrichmentPartial(3, 10),
	}
	for _, d := range all {
		if d.Kind == "" {
			t.Errorf("disclosure has no kind: %+v", d)
		}
		if strings.TrimSpace(d.Reason) == "" {
			t.Errorf("disclosure %q has no reason", d.Kind)
		}
	}
}

// TestBoundedDisclosuresCarryANextAction: a bound the caller can act on must say
// how.
func TestBoundedDisclosuresCarryANextAction(t *testing.T) {
	t.Parallel()
	actionable := []model.Disclosure{
		activity.BudgetBounded(12, 12),
		activity.QuotaExhausted("2026-07-25T14:32:00Z"),
		activity.QuotaExhausted(""),
		activity.EnrichmentPartial(3, 10),
		activity.ProviderCapped(),
		activity.PatchTruncated(),
		activity.AncestryDiverged(),
	}
	for _, d := range actionable {
		if strings.TrimSpace(d.NextAction) == "" {
			t.Errorf("disclosure %q states a limit but no next action", d.Kind)
		}
	}
}

func TestReferenceScopedNamesTheReference(t *testing.T) {
	t.Parallel()
	if got := activity.ReferenceScoped("release/1.x"); !strings.Contains(got.Reason, "release/1.x") {
		t.Errorf("reason does not name the reference: %q", got.Reason)
	}
	// An unresolved reference still has to read sensibly.
	if got := activity.ReferenceScoped(""); !strings.Contains(got.Reason, "default branch") {
		t.Errorf("empty reference not labeled: %q", got.Reason)
	}
}

func TestBudgetBoundedReportsTheNumbers(t *testing.T) {
	t.Parallel()
	got := activity.BudgetBounded(37, 40)
	if !strings.Contains(got.Reason, "40") || !strings.Contains(got.Reason, "37") {
		t.Errorf("reason omits the ceiling or the consumption: %q", got.Reason)
	}
}

func TestQuotaExhaustedIncludesResetWhenKnown(t *testing.T) {
	t.Parallel()
	withReset := activity.QuotaExhausted("2026-07-25T14:32:00Z")
	if !strings.Contains(withReset.Reason, "2026-07-25T14:32:00Z") {
		t.Errorf("reason omits the reset time: %q", withReset.Reason)
	}
	// Without one it must still be a complete, actionable sentence.
	without := activity.QuotaExhausted("")
	if strings.Contains(without.Reason, "resets at .") {
		t.Errorf("reason has a dangling reset clause: %q", without.Reason)
	}
}

func TestEnrichmentPartialReportsBothCounts(t *testing.T) {
	t.Parallel()
	got := activity.EnrichmentPartial(3, 10)
	if !strings.Contains(got.Reason, "3") || !strings.Contains(got.Reason, "10") {
		t.Errorf("reason omits delivered or requested: %q", got.Reason)
	}
}

func TestAuthorFilterNotAppliedNamesTheAuthor(t *testing.T) {
	t.Parallel()
	if got := activity.AuthorFilterNotApplied("octocat"); !strings.Contains(got.Reason, "octocat") {
		t.Errorf("reason does not name the author: %q", got.Reason)
	}
}
