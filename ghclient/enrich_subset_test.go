// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"testing"

	"github.com/skaphos/sting/model"
)

// TestObservedAttributionRequiresEnrichment is the highest-value test in the
// feature (FR-017).
//
// It asserts the single invariant the whole attribution story rests on:
// Correlation.Basis == "observed" requires Enriched == true on every SHA it
// names. If this breaks, the tool starts presenting inference as observation —
// the one thing the basis label exists to prevent.
func TestObservedAttributionRequiresEnrichment(t *testing.T) {
	const commits = 20
	tr := newCountingTransport(commits, 100)
	c := budgetedClientWithTransport(t, tr, 100, model.DefaultMaxRequests)

	q := activityQuery("skaphos/sting")
	q.Ref = "main"
	q.EnrichCommits = 5

	res, err := c.CollectActivity(context.Background(), q)
	if err != nil {
		t.Fatalf("CollectActivity: %v", err)
	}

	enriched := map[string]bool{}
	for _, cm := range res.Commits {
		if cm.Enriched {
			enriched[cm.SHA] = true
		}
	}

	observedSHAs := map[string]bool{}
	for _, corr := range res.Correlations {
		if corr.Basis != model.BasisObserved {
			continue
		}
		for _, sha := range corr.SHAs {
			observedSHAs[sha] = true
			if !enriched[sha] {
				t.Errorf("correlation for %q claims observed provenance for %s, "+
					"which was never enriched", corr.Path, sha)
			}
		}
	}

	// At most the requested subset can carry observed attribution.
	if len(observedSHAs) > q.EnrichCommits {
		t.Errorf("%d distinct SHAs carry observed attribution, want at most %d",
			len(observedSHAs), q.EnrichCommits)
	}

	// Guard against the assertion above passing vacuously: if enrichment
	// produced no observed correlation at all, the loop never ran and the
	// invariant was never actually exercised.
	if len(observedSHAs) == 0 {
		t.Fatal("no observed correlations were produced, so the invariant was not exercised; " +
			"the fixture must return per-commit files for enriched commits")
	}
	if len(enriched) != q.EnrichCommits {
		t.Errorf("enriched %d commits, want %d", len(enriched), q.EnrichCommits)
	}
}

// TestNoEnrichmentMeansNoObservedAttribution: without per-commit data, nothing
// can honestly be called observed.
func TestNoEnrichmentMeansNoObservedAttribution(t *testing.T) {
	tr := newCountingTransport(10, 100)
	c := budgetedClientWithTransport(t, tr, 100, model.DefaultMaxRequests)

	q := activityQuery("skaphos/sting")
	q.Ref = "main"
	// EnrichCommits deliberately left at 0.

	res, err := c.CollectActivity(context.Background(), q)
	if err != nil {
		t.Fatalf("CollectActivity: %v", err)
	}

	for _, cm := range res.Commits {
		if cm.Enriched {
			t.Errorf("commit %s marked Enriched without enrichment being requested", cm.SHA)
		}
	}
	for _, corr := range res.Correlations {
		if corr.Basis == model.BasisObserved {
			t.Errorf("correlation for %q claims observed provenance with no enrichment requested",
				corr.Path)
		}
	}
	_, byKind := tr.counts()
	if got := byKind["commit-detail"]; got != 0 {
		t.Errorf("issued %d per-commit requests without --enrich-commits, want 0", got)
	}
}

// TestEnrichmentSubsetIsTheFirstNInOrder is the determinism half of research
// R3: the subset is clipped by commit order, never by completion order, so the
// same query enriches the same commits every run.
func TestEnrichmentSubsetIsTheFirstNInOrder(t *testing.T) {
	const (
		commits = 30
		subset  = 6
	)
	run := func() []string {
		tr := newCountingTransport(commits, 100)
		c := budgetedClientWithTransport(t, tr, 100, model.DefaultMaxRequests)
		q := activityQuery("skaphos/sting")
		q.Ref = "main"
		q.EnrichCommits = subset

		res, err := c.CollectActivity(context.Background(), q)
		if err != nil {
			t.Fatalf("CollectActivity: %v", err)
		}
		var got []string
		for _, cm := range res.Commits {
			if cm.Enriched {
				got = append(got, cm.SHA)
			}
		}
		return got
	}

	first := run()
	if len(first) != subset {
		t.Fatalf("enriched %d commits, want %d", len(first), subset)
	}

	// The enriched commits must be exactly the first N of the result, not an
	// arbitrary set that happened to win a race.
	tr := newCountingTransport(commits, 100)
	c := budgetedClientWithTransport(t, tr, 100, model.DefaultMaxRequests)
	q := activityQuery("skaphos/sting")
	q.Ref = "main"
	q.EnrichCommits = subset
	res, err := c.CollectActivity(context.Background(), q)
	if err != nil {
		t.Fatalf("CollectActivity: %v", err)
	}
	for i, cm := range res.Commits {
		wantEnriched := i < subset
		if cm.Enriched != wantEnriched {
			t.Errorf("commit %d (%s): Enriched = %v, want %v — the subset must be the "+
				"first %d in order", i, cm.SHA, cm.Enriched, wantEnriched, subset)
		}
	}

	// Repeated runs must enrich the identical set, in the identical order.
	for i := range 5 {
		got := run()
		if len(got) != len(first) {
			t.Fatalf("run %d enriched %d commits, want %d", i, len(got), len(first))
		}
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("run %d enriched %v, want %v — enrichment is not deterministic",
					i, got, first)
			}
		}
	}
}

// TestEnrichmentChecksBudgetBeforeDispatch is the other half of R3: when
// capacity is short the batch is trimmed to what can be afforded, rather than
// firing requests and letting the losers fail. Fire-and-fail would make *which*
// commits got enriched depend on which requests won the race.
func TestEnrichmentChecksBudgetBeforeDispatch(t *testing.T) {
	const commits = 50
	tr := newCountingTransport(commits, 100)
	// 1 pre-flight + 1 listing page + 1 comparison leaves very little for
	// enrichment, so the subset must be trimmed rather than attempted in full.
	c := budgetedClientWithTransport(t, tr, 100, 6)

	q := activityQuery("skaphos/sting")
	q.Ref = "main"
	q.EnrichCommits = 40 // far more than the budget allows

	res, err := c.CollectActivity(context.Background(), q)
	if err != nil {
		t.Fatalf("a short budget must not be an error: %v", err)
	}

	_, byKind := tr.counts()
	if res.Cost.Consumed > 6 {
		t.Errorf("Consumed = %d, exceeded the ceiling of 6", res.Cost.Consumed)
	}
	// The change set must still have been produced: enrichment reserves
	// capacity for the comparison rather than starving it.
	if byKind["compare"] != 1 {
		t.Errorf("comparison requests = %d, want 1 — enrichment starved the change set",
			byKind["compare"])
	}
	// Fewer than requested were enriched, and the result says so.
	enriched := 0
	for _, cm := range res.Commits {
		if cm.Enriched {
			enriched++
		}
	}
	if enriched >= 40 {
		t.Errorf("enriched %d commits within a ceiling of 6", enriched)
	}
	if !hasDisclosure(res.Disclosures, model.DisclosureEnrichmentPartial) {
		t.Errorf("a trimmed enrichment subset produced no enrichment-partial disclosure; got %v",
			disclosureKinds(res.Disclosures))
	}
}

// TestEnrichmentRequestCountMatchesSubset: enrichment costs exactly one request
// per commit, so the documented price is the real one.
func TestEnrichmentRequestCountMatchesSubset(t *testing.T) {
	const subset = 7
	tr := newCountingTransport(40, 100)
	c := budgetedClientWithTransport(t, tr, 100, model.DefaultMaxRequests)

	q := activityQuery("skaphos/sting")
	q.Ref = "main"
	q.EnrichCommits = subset

	if _, err := c.CollectActivity(context.Background(), q); err != nil {
		t.Fatalf("CollectActivity: %v", err)
	}
	_, byKind := tr.counts()
	if got := byKind["commit-detail"]; got != subset {
		t.Errorf("per-commit requests = %d, want exactly %d (one per enriched commit)", got, subset)
	}
}
