// SPDX-License-Identifier: MIT
package render

import (
	"fmt"
	"strings"

	"github.com/skaphos/sting/model"
)

// RenderPRInbox renders the independent current-work envelope.
func RenderPRInbox(r model.PRInboxResult, f Format) (string, error) {
	switch f {
	case FormatJSON:
		return prJSON(r)
	case FormatMarkdown:
		return PRInboxMarkdown(r), nil
	default:
		return "", fmt.Errorf("unknown format %q", f)
	}
}

// PRInboxMarkdown displays current inclusion reasons without implying an activity window.
func PRInboxMarkdown(r model.PRInboxResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Personal PR inbox\n\n- User: %s (%s)\n- No time restriction\n- Scope: %s; targets: %s; org: %s\n- Current state: %s; drafts: %s; PR limit: %d (0 = uncapped)\n", codeSpan(r.Query.User), r.Query.IdentitySource, r.Query.Scope, codeSpan(strings.Join(r.Query.Repos, ", ")), codeSpan(r.Query.Org), r.Query.State, r.Query.Draft, r.Query.MaxPRs)
	org, repo := "", ""
	for _, entry := range r.PRs {
		prHeading(&b, entry.PR, &org, &repo)
		prSummary(&b, entry.PR)
		reasons := []string{}
		for _, reason := range entry.Reasons {
			reasons = append(reasons, string(reason))
		}
		fmt.Fprintf(&b, "  - Reasons: %s; complete: %t\n", strings.Join(reasons, ", "), entry.ReasonsComplete)
	}
	prFooter(&b, r.Count, r.Cost, r.Coverage, r.Disclosures)
	return b.String()
}
