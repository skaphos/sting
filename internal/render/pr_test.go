// SPDX-License-Identifier: MIT
package render

import (
	"strings"
	"testing"
	"time"

	"github.com/skaphos/sting/model"
)

func TestPRRendering(t *testing.T) {
	at := time.Date(2026, 8, 24, 13, 0, 0, 0, time.UTC)
	state := model.PRStateMerged
	draft := false
	r := model.PRResult{SchemaVersion: model.PRSchemaVersion, Workflow: "pr-activity", Provider: model.ProviderGitHub, Query: model.PRQuery{Since: at, Until: at, TimeBasis: model.PRTimeAny, State: model.PRStateAll}, Count: 1, PRs: []model.PRActivityEntry{{PR: model.PullRequest{Repo: "acme/api", Number: 7, Title: "A `title`", URL: "https://github.com/acme/api/pull/7", State: &state, Draft: &draft}, Matches: []model.PRMatch{{ID: "event:1", Kind: "closed", At: at, Source: "issue-event"}}}}, Cost: model.PRCostReport{Consumed: 2, Ceiling: 10}, Disclosures: []model.PRDisclosure{{Kind: "history-incomplete", Reason: "budget reached", Repo: "acme/api", Number: 7}}}
	md := PRsMarkdown(r)
	for _, want := range []string{"acme", "api", "closed", "merged", "2026-08-24", "history-incomplete", "2", "10"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in %s", want, md)
		}
	}
	if _, err := RenderPRs(r, FormatJSON); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderPRs(r, Format("bad")); err == nil {
		t.Fatal("accepted bad format")
	}
}
