// SPDX-License-Identifier: MIT
package render

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/skaphos/sting/model"
)

// RenderPRs renders the complete activity result or its Markdown view.
func RenderPRs(r model.PRResult, f Format) (string, error) {
	switch f {
	case FormatJSON:
		return prJSON(r)
	case FormatMarkdown:
		return PRsMarkdown(r), nil
	default:
		return "", fmt.Errorf("unknown format %q", f)
	}
}
func prJSON(r any) (string, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode PR JSON: %w", err)
	}
	return string(b), nil
}

// PRsMarkdown displays only facts present in the structured result.
func PRsMarkdown(r model.PRResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# PR activity\n\n- Window: %s → %s\n- Scope: %s; targets: %s; org: %s\n- Author: %s; role: %s; current state: %s; drafts: %s; time basis: %s\n- PR limit: %d (0 = uncapped)\n", r.Query.Since.UTC().Format(time.RFC3339Nano), r.Query.Until.UTC().Format(time.RFC3339Nano), r.Query.Scope, codeSpan(strings.Join(r.Query.Repos, ", ")), codeSpan(r.Query.Org), codeSpan(r.Query.Author), r.Query.Role, r.Query.State, r.Query.Draft, r.Query.TimeBasis, r.Query.MaxPRs)
	org, repo := "", ""
	for _, entry := range r.PRs {
		prHeading(&b, entry.PR, &org, &repo)
		prSummary(&b, entry.PR)
		for _, m := range entry.Matches {
			fmt.Fprintf(&b, "  - %s at %s (%s, %s)\n", m.Kind, m.At.UTC().Format(time.RFC3339Nano), m.Source, codeSpan(m.ID))
		}
		fmt.Fprintf(&b, "  - Matching evidence complete: %t\n", entry.MatchesComplete)
	}
	prFooter(&b, r.Count, r.Cost, r.Coverage, r.Disclosures)
	return b.String()
}
func prHeading(b *strings.Builder, p model.PullRequest, org, repo *string) {
	owner, _, _ := strings.Cut(p.Repo, "/")
	if owner != *org {
		fmt.Fprintf(b, "\n## %s\n", codeSpan(owner))
		*org = owner
		*repo = ""
	}
	if p.Repo != *repo {
		fmt.Fprintf(b, "\n### %s\n\n", codeSpan(p.Repo))
		*repo = p.Repo
	}
}
func prSummary(b *strings.Builder, p model.PullRequest) {
	state := "unknown"
	if p.State != nil {
		state = string(*p.State)
	}
	link := fmt.Sprintf("#%d", p.Number)
	if p.URL != "" {
		target := strings.NewReplacer("<", "%3C", ">", "%3E", "\n", "%0A", "\r", "%0D").Replace(p.URL)
		link = fmt.Sprintf("[#%d](<%s>)", p.Number, target)
	}
	fmt.Fprintf(b, "- %s %s [%s]", link, codeSpan(p.Title), state)
	if p.Draft != nil && *p.Draft {
		b.WriteString(" [draft]")
	}
	if len(p.RequestedReviewers) > 0 {
		b.WriteString(" [pending review request]")
	}
	fmt.Fprintf(b, " — %s\n", codeSpan(p.URL))
	if len(p.MissingFields) > 0 {
		fmt.Fprintf(b, "  - Unavailable: %s\n", codeSpan(strings.Join(p.MissingFields, ", ")))
	}
}
func prFooter(b *strings.Builder, count int, cost model.PRCostReport, coverage model.PRCoverage, disclosures []model.PRDisclosure) {
	fmt.Fprintf(b, "\n## Coverage and cost\n\n- PRs: %d\n- Discovery complete: %t; matching evidence complete: %t\n- Requests: %d / %d (0 ceiling = uncapped); identity: %d; discovery: %d; history: %d\n", count, coverage.DiscoveryComplete, coverage.EvidenceComplete, cost.Consumed, cost.Ceiling, cost.IdentityRequests, cost.DiscoveryRequests, cost.HistoryRequests)
	for _, d := range disclosures {
		fmt.Fprintf(b, "- %s: %s", codeSpan(d.Kind), codeSpan(d.Reason))
		if d.Repo != "" {
			fmt.Fprintf(b, " (%s", codeSpan(d.Repo))
			if d.Number > 0 {
				fmt.Fprintf(b, " #%d", d.Number)
			}
			b.WriteString(")")
		}
		if d.Stream != "" {
			fmt.Fprintf(b, " [stream %s]", codeSpan(d.Stream))
		}
		if d.NextAction != "" {
			fmt.Fprintf(b, " Next: %s", codeSpan(d.NextAction))
		}
		b.WriteByte('\n')
	}
}
