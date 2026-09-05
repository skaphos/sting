// SPDX-License-Identifier: MIT
package render

import (
	"strings"
	"testing"

	"github.com/skaphos/sting/model"
)

func TestPRInboxRendering(t *testing.T) {
	r := model.PRInboxResult{Query: model.PRInboxQuery{User: "octocat"}, PRs: []model.PRInboxEntry{{PR: model.PullRequest{Repo: "acme/api", Number: 1}, Reasons: []model.PRRelationship{model.PRAuthored, model.PRAssigned, model.PRReviewRequested}}}}
	md := PRInboxMarkdown(r)
	for _, s := range []string{"octocat", "authored", "assigned", "review-requested", "No time restriction"} {
		if !strings.Contains(md, s) {
			t.Errorf("missing %s", s)
		}
	}
	if strings.Contains(md, "Window:") {
		t.Fatal("inbox inherited window")
	}
	if _, e := RenderPRInbox(r, FormatJSON); e != nil {
		t.Fatal(e)
	}
	if _, e := RenderPRInbox(r, Format("bad")); e == nil {
		t.Fatal("invalid format accepted")
	}
}
