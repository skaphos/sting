// SPDX-License-Identifier: MIT
package render_test

import (
	"strings"
	"testing"

	"github.com/skaphos/sting/internal/render"
	"github.com/skaphos/sting/model"
)

func TestActivityIncompleteIsNotObservedAbsence(t *testing.T) {
	for _, kind := range []string{model.DisclosureQuotaExhausted, model.DisclosureBudgetBounded, model.DisclosureCollectionFailed} {
		for _, listed := range []bool{false, true} {
			r := model.ActivityResult{Repo: "a/b", CommitsCollected: listed,
				Disclosures: []model.Disclosure{{Kind: kind, Reason: "stopped early"}}}
			md := render.ActivityMarkdown(r)
			if strings.Contains(md, "No file changes between") || (!listed && strings.Contains(md, "No commits in this window")) {
				t.Fatalf("uncollected evidence claimed absent: %s", md)
			}
			if !strings.Contains(md, "not collected") || !strings.Contains(md, kind) {
				t.Fatalf("missing stage or disclosure: %s", md)
			}
		}
	}
}

func TestActivityEstimateRetainsDisclosures(t *testing.T) {
	r := model.ActivityResult{EstimateOnly: true, Cost: model.CostReport{Consumed: 1},
		Disclosures: []model.Disclosure{{Kind: model.DisclosureQuotaExhausted, Reason: "quota stopped probe"}}}
	md := render.ActivityMarkdown(r)
	if !strings.Contains(md, "No evidence was gathered") || !strings.Contains(md, "quota stopped probe") || strings.Contains(md, "## Commits") {
		t.Fatalf("estimate misrendered: %s", md)
	}
}
