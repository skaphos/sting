// SPDX-License-Identifier: MIT
package ghclient

import (
	"context"
	"testing"

	"github.com/skaphos/sting/model"
)

// TestCollectActivityBudgetStopReturnsPartialResult is Constitution VI in
// executable form: a ceiling reached mid-listing must return the commits
// gathered so far plus an attributable disclosure — and **no** Go error.
//
// Returning an error here would discard evidence the caller can use, which is
// exactly the blindness the principle forbids.
func TestCollectActivityBudgetStopReturnsPartialResult(t *testing.T) {
	const (
		commits = 500
		perPage = 100
		// 1 pre-flight + 1 listing page, then the ceiling bites mid-pagination.
		ceiling = 2
	)
	tr := newCountingTransport(commits, perPage)
	c := budgetedClientWithTransport(t, tr, perPage, ceiling)

	q := activityQuery("skaphos/sting")
	q.Ref = "main" // no default-branch lookup, so the arithmetic is predictable

	res, err := c.CollectActivity(context.Background(), q)
	if err != nil {
		t.Fatalf("a budget stop must not be an error: %v", err)
	}

	// Whatever was gathered before the stop must survive.
	if res.Count == 0 {
		t.Error("Count = 0: the commits gathered before the ceiling were discarded")
	}
	if res.Count >= commits {
		t.Errorf("Count = %d, want fewer than %d — the ceiling should have stopped listing",
			res.Count, commits)
	}

	if !hasDisclosure(res.Disclosures, model.DisclosureBudgetBounded) {
		t.Errorf("missing budget-bounded disclosure; got %v", disclosureKinds(res.Disclosures))
	}
	for _, d := range res.Disclosures {
		if d.Kind == model.DisclosureBudgetBounded {
			if d.Reason == "" {
				t.Error("budget-bounded disclosure carries no reason")
			}
			if d.NextAction == "" {
				t.Error("budget-bounded disclosure carries no next action")
			}
		}
	}

	// The cost report must reflect what was really consumed, not the ceiling.
	if res.Cost.Consumed > ceiling {
		t.Errorf("Consumed = %d, exceeds the ceiling of %d", res.Cost.Consumed, ceiling)
	}
	if res.Cost.Ceiling != ceiling {
		t.Errorf("Cost.Ceiling = %d, want %d", res.Cost.Ceiling, ceiling)
	}
	// The result stays well-formed even though it was cut short.
	if res.SchemaVersion != model.ActivitySchemaVersion {
		t.Errorf("SchemaVersion = %q on a bounded result", res.SchemaVersion)
	}
}

// TestCollectActivityBudgetStopBeforeAnything covers the extreme: a ceiling so
// low that nothing can be gathered. Even then the result must be well-formed
// and explain itself rather than surfacing as an opaque failure.
func TestCollectActivityBudgetStopImmediately(t *testing.T) {
	tr := newCountingTransport(100, 100)
	c := budgetedClientWithTransport(t, tr, 100, 1)

	res, err := c.CollectActivity(context.Background(), activityQuery("skaphos/sting"))
	if err != nil {
		t.Fatalf("an immediate budget stop must not be an error: %v", err)
	}
	if res.SchemaVersion != model.ActivitySchemaVersion {
		t.Errorf("SchemaVersion = %q, want it populated even when nothing was gathered", res.SchemaVersion)
	}
	if len(res.Disclosures) == 0 {
		t.Error("a query that gathered nothing must say why")
	}
	// Cost is populated on every path, including this one.
	if res.Cost.Ceiling != 1 {
		t.Errorf("Cost.Ceiling = %d, want 1", res.Cost.Ceiling)
	}
}

// TestCollectActivityUncappedRunIsNotStopped confirms the ceiling of 0 really
// means uncapped: accounting stays on, enforcement stays off.
func TestCollectActivityUncappedRunIsNotStopped(t *testing.T) {
	const commits = 500
	tr := newCountingTransport(commits, 100)
	c := budgetedClientWithTransport(t, tr, 100, 0)

	q := activityQuery("skaphos/sting")
	q.Ref = "main"

	res, err := c.CollectActivity(context.Background(), q)
	if err != nil {
		t.Fatalf("CollectActivity: %v", err)
	}
	if res.Count != commits {
		t.Errorf("Count = %d, want all %d commits on an uncapped run", res.Count, commits)
	}
	if hasDisclosure(res.Disclosures, model.DisclosureBudgetBounded) {
		t.Error("an uncapped run reported a budget bound")
	}
	if res.Cost.Consumed == 0 {
		t.Error("Consumed = 0: accounting must stay on even when enforcement is off")
	}
}

// TestCollectActivityGenerousCeilingCompletes guards the other direction — the
// default ceiling must not clip an ordinary query.
func TestCollectActivityGenerousCeilingCompletes(t *testing.T) {
	const commits = 250
	tr := newCountingTransport(commits, 100)
	c := budgetedClientWithTransport(t, tr, 100, model.DefaultMaxRequests)

	res, err := c.CollectActivity(context.Background(), activityQuery("skaphos/sting"))
	if err != nil {
		t.Fatalf("CollectActivity: %v", err)
	}
	if res.Count != commits {
		t.Errorf("Count = %d, want %d: the default ceiling clipped an ordinary query", res.Count, commits)
	}
	if hasDisclosure(res.Disclosures, model.DisclosureBudgetBounded) {
		t.Errorf("the default ceiling of %d bounded a %d-commit window",
			model.DefaultMaxRequests, commits)
	}
}

// TestEstimateOnlyGathersNothing is FR-011: an estimate reports projected cost
// without collecting evidence.
func TestEstimateOnlyGathersNothing(t *testing.T) {
	tr := newCountingTransport(250, 100)
	c := budgetedClientWithTransport(t, tr, 100, model.DefaultMaxRequests)

	q := activityQuery("skaphos/sting")
	q.Ref = "main"
	q.EstimateOnly = true

	res, err := c.CollectActivity(context.Background(), q)
	if err != nil {
		t.Fatalf("CollectActivity(estimate): %v", err)
	}

	if res.Count != 0 || len(res.Commits) != 0 {
		t.Errorf("an estimate gathered %d commits; it must gather none", res.Count)
	}
	if len(res.ChangeSet.Paths) != 0 {
		t.Error("an estimate produced a change set")
	}
	if res.Cost.Estimated == 0 {
		t.Error("an estimate reported no projected cost")
	}

	_, byKind := tr.counts()
	if got := byKind["compare"]; got != 0 {
		t.Errorf("estimate issued %d comparison requests, want 0", got)
	}
	if got := byKind["list-commits"]; got != 1 {
		t.Errorf("estimate issued %d list-commits requests, want exactly 1 (the probe)", got)
	}
}

// TestPreflightQuotaExhausted is FR-016: when the quota is already spent, say so
// before gathering rather than after discovering it the expensive way.
func TestPreflightQuotaExhausted(t *testing.T) {
	tr := newCountingTransport(100, 100)
	tr.exhaustedQuota = true
	c := budgetedClientWithTransport(t, tr, 100, model.DefaultMaxRequests)

	res, err := c.CollectActivity(context.Background(), activityQuery("skaphos/sting"))
	if err != nil {
		t.Fatalf("an exhausted quota must not be a bare error: %v", err)
	}
	if !hasDisclosure(res.Disclosures, model.DisclosureQuotaExhausted) {
		t.Errorf("missing quota-exhausted disclosure; got %v", disclosureKinds(res.Disclosures))
	}

	_, byKind := tr.counts()
	if got := byKind["list-commits"] + byKind["compare"]; got != 0 {
		t.Errorf("issued %d evidence requests despite an exhausted quota, want 0", got)
	}
	if res.SchemaVersion != model.ActivitySchemaVersion {
		t.Error("the result is not well-formed")
	}
}
