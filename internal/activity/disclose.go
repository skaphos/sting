// SPDX-License-Identifier: MIT
// Package activity holds the provider-agnostic logic of the repository
// activity digest: the disclosure builders that state what a result does not
// cover, and the deterministic rules that correlate changed paths with commits.
//
// It deliberately depends on nothing but model, so the honesty rules can be
// unit-tested without a provider in the picture.
package activity

import (
	"fmt"

	"github.com/skaphos/sting/model"
)

// DisclosureInput describes what a collected result did and did not cover, so
// the corresponding disclosures can be built in one place.
type DisclosureInput struct {
	// Ref is the reference the change set was scoped to. Never empty by the
	// time disclosures are built — the resolved default branch is filled in.
	Ref string
	// ChangeSetProduced is true when a boundary comparison was presented.
	// The two unconditional disclosures hang off this.
	ChangeSetProduced bool
	// Diverged is true when the boundaries do not share ancestry.
	Diverged bool
	// ProviderCapped is true when the comparison hit the provider's file cap.
	ProviderCapped bool
	// PatchTruncated is true when any patch text exceeded the byte budget.
	PatchTruncated bool
	// AuthorFilter is the author the commit listing was narrowed to, if any.
	AuthorFilter string
	// RootCommitBase is true when the window reaches the repository's root
	// commit, so there is no parent to compare against.
	RootCommitBase bool
}

// Build returns the disclosures for a collected result, in a fixed order so
// identical upstream state yields byte-identical output.
//
// Two of them — reference-scoped and net-comparison-blindspot — are
// unconditional whenever a change set is produced, including on a completely
// clean result. A boundary comparison always carries those blind spots, so
// stating them only when something went wrong would let a reader treat the
// ordinary case as more complete than it is.
func Build(in DisclosureInput) []model.Disclosure {
	var out []model.Disclosure

	if in.ChangeSetProduced {
		blindspot := NetComparisonBlindspot()
		if in.RootCommitBase {
			// One disclosure per kind: the root-commit caveat is a facet of the
			// same blind spot, so it extends that reason rather than emitting a
			// second entry under an identical kind.
			blindspot = RepositoryRootBase()
		}
		out = append(out, ReferenceScoped(in.Ref), blindspot)
	}
	if in.Diverged {
		out = append(out, AncestryDiverged())
	}
	if in.ProviderCapped {
		out = append(out, ProviderCapped())
	}
	if in.PatchTruncated {
		out = append(out, PatchTruncated())
	}
	if in.AuthorFilter != "" && in.ChangeSetProduced {
		out = append(out, AuthorFilterNotApplied(in.AuthorFilter))
	}
	return out
}

// ReferenceScoped names the single reference the result covers. Unconditional.
func ReferenceScoped(ref string) model.Disclosure {
	if ref == "" {
		ref = "the default branch"
	}
	return model.Disclosure{
		Kind: model.DisclosureReferenceScoped,
		Reason: fmt.Sprintf("This covers %s only. Work on other branches, forks, or unmerged "+
			"pull requests is not included.", ref),
		NextAction: "Re-run with --ref to examine a different branch or tag.",
	}
}

// NetComparisonBlindspot states what a boundary comparison structurally cannot
// see. Unconditional whenever a change set is produced.
func NetComparisonBlindspot() model.Disclosure {
	return model.Disclosure{
		Kind: model.DisclosureNetComparisonBlindspot,
		Reason: "The change set compares the window's start and end states. A file created and " +
			"deleted inside the window, or edited then reverted, does not appear, and " +
			"intermediate revisions of a file are collapsed into one net change.",
		NextAction: "Read the commit list for work the net comparison collapses, or narrow the window.",
	}
}

// AncestryDiverged reports boundaries that do not share ancestry, which happens
// after a force-push or a rebase inside the window.
func AncestryDiverged() model.Disclosure {
	return model.Disclosure{
		Kind: model.DisclosureAncestryDiverged,
		Reason: "The window's start and end commits do not share ancestry, so a net change set " +
			"would be meaningless. History was rewritten (a force-push or a rebase) inside " +
			"the window.",
		NextAction: "Narrow the window to a range after the rewrite, or compare an explicit ref.",
	}
}

// RepositoryRootBase is the net-comparison blind spot as it applies to a window
// reaching back to the repository's first commit: there is no parent to compare
// against, so the comparison starts at the root commit itself and the files that
// commit introduced fall outside it.
func RepositoryRootBase() model.Disclosure {
	return model.Disclosure{
		Kind: model.DisclosureNetComparisonBlindspot,
		Reason: "The change set compares the window's start and end states, so a file created and " +
			"deleted inside the window does not appear. This window also reaches the " +
			"repository's root commit, which has no parent: the change set is measured from " +
			"that commit, so the files it introduced are not counted as changes.",
		NextAction: "Treat the root commit's own contents as pre-existing state, and read the " +
			"commit list for work the net comparison collapses.",
	}
}

// ProviderCapped reports that the provider clipped the comparison's file list.
func ProviderCapped() model.Disclosure {
	return model.Disclosure{
		Kind: model.DisclosureProviderCapped,
		Reason: "The provider caps a comparison at 300 files, and this window exceeded it. The " +
			"change set is incomplete.",
		NextAction: "Split the window into shorter ranges and combine the results.",
	}
}

// PatchTruncated reports that patch text was clipped by the byte budget.
func PatchTruncated() model.Disclosure {
	return model.Disclosure{
		Kind:       model.DisclosurePatchTruncated,
		Reason:     "Patch text exceeded the configured byte budget and was truncated.",
		NextAction: "Raise --max-diff-bytes, or drop --include-diffs and read the paths alone.",
	}
}

// AuthorFilterNotApplied states the asymmetry between an author-filtered commit
// list and a change set that cannot be author-filtered: a boundary comparison
// has no notion of authorship.
func AuthorFilterNotApplied(author string) model.Disclosure {
	return model.Disclosure{
		Kind: model.DisclosureAuthorFilterNotApplied,
		Reason: fmt.Sprintf("The commit list is filtered to %s, but the change set is not: a "+
			"boundary comparison has no notion of authorship, so it covers every author "+
			"who touched the reference in this window.", author),
		NextAction: "Use --enrich-commits to attribute paths to specific commits.",
	}
}

// BudgetBounded reports that the request ceiling stopped evidence gathering.
// The result it accompanies is still a result: what was gathered is returned.
func BudgetBounded(consumed, ceiling int) model.Disclosure {
	return model.Disclosure{
		Kind: model.DisclosureBudgetBounded,
		Reason: fmt.Sprintf("The request ceiling of %d was reached after %d requests, so gathering "+
			"stopped early. What had been collected is reported.", ceiling, consumed),
		NextAction: "Raise --max-requests, or narrow the window to fit the current ceiling.",
	}
}

// QuotaExhausted reports that the provider's rate limit stopped gathering.
func QuotaExhausted(resetsAt string) model.Disclosure {
	reason := "The provider's rate limit was reached, so gathering stopped early. What had been " +
		"collected is reported."
	next := "Wait for the quota to reset, then re-run."
	if resetsAt != "" {
		reason = fmt.Sprintf("The provider's rate limit was reached, so gathering stopped early. "+
			"What had been collected is reported. The quota resets at %s.", resetsAt)
		next = fmt.Sprintf("Wait until %s for the quota to reset, then re-run.", resetsAt)
	}
	return model.Disclosure{
		Kind:       model.DisclosureQuotaExhausted,
		Reason:     reason,
		NextAction: next,
	}
}

// EnrichmentPartial reports that fewer commits were enriched than requested, so
// a reader knows the observed attributions cover only part of the window.
func EnrichmentPartial(delivered, requested int) model.Disclosure {
	return model.Disclosure{
		Kind: model.DisclosureEnrichmentPartial,
		Reason: fmt.Sprintf("Per-commit detail was requested for %d commits but only %d could be "+
			"fetched within the request budget. Paths touched only by the remaining "+
			"commits carry inferred attribution at best.", requested, delivered),
		NextAction: "Raise --max-requests to enrich the full subset.",
	}
}
