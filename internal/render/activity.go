// SPDX-License-Identifier: MIT
package render

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/skaphos/sting/model"
)

// RenderActivity encodes a repository activity result in the requested format.
// JSON is the contract; Markdown is a view of it, never a superset.
func RenderActivity(r model.ActivityResult, f Format) (string, error) {
	switch f {
	case FormatJSON:
		return activityJSON(r)
	case FormatMarkdown:
		return activityMarkdown(r), nil
	default:
		return "", fmt.Errorf("unknown format %q", f)
	}
}

// ActivityMarkdown renders r as Markdown. It never fails, so it is convenient
// for callers that always want the human-readable form (the MCP text content,
// for one).
func ActivityMarkdown(r model.ActivityResult) string {
	return activityMarkdown(r)
}

// ActivityEstimate renders estimate-only output: the projected cost and the
// remaining quota, with an explicit statement that nothing was gathered.
//
// Saying so plainly matters — an estimate that reads like a result would let a
// caller believe it had evidence it never collected.
func ActivityEstimate(cost model.CostReport) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Estimated cost: %d provider requests.\n", cost.Estimated)

	if cost.Ceiling > 0 {
		fmt.Fprintf(&b, "Ceiling: %d.", cost.Ceiling)
	} else {
		b.WriteString("Ceiling: none.")
	}
	if cost.QuotaLimit > 0 {
		fmt.Fprintf(&b, " Quota: %d of %d remaining", cost.QuotaRemaining, cost.QuotaLimit)
		if !cost.QuotaResetsAt.IsZero() {
			fmt.Fprintf(&b, ", resets %s", cost.QuotaResetsAt.UTC().Format("15:04Z"))
		}
		b.WriteString(".")
	}
	b.WriteString("\n")

	if cost.Ceiling > 0 && cost.Estimated > cost.Ceiling {
		fmt.Fprintf(&b, "This exceeds the ceiling of %d and would be stopped early.\n", cost.Ceiling)
	}

	fmt.Fprintf(&b, "The estimate itself consumed %d request(s).\n", cost.Consumed)
	b.WriteString("No evidence was gathered. Re-run without --estimate to collect it.\n")

	return b.String()
}

func activityJSON(r model.ActivityResult) (string, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode json: %w", err)
	}
	return string(b), nil
}

func activityMarkdown(r model.ActivityResult) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Activity: %s\n\n", codeSpan(r.Repo))
	writeActivityHeader(&b, r)
	writeActivityCommits(&b, r)
	writeActivityChangeSet(&b, r)
	writeActivityCorrelations(&b, r)
	writeActivityCost(&b, r.Cost)
	writeActivityDisclosures(&b, r.Disclosures)

	return b.String()
}

// writeActivityHeader echoes the resolved query and the boundaries, which is
// what makes the result re-derivable rather than merely readable.
func writeActivityHeader(b *strings.Builder, r model.ActivityResult) {
	ref := r.Ref
	if ref == "" {
		ref = "(default branch)"
	}
	fmt.Fprintf(b, "**Reference**: %s\n", codeSpan(ref))
	fmt.Fprintf(b, "**Window**: %s .. %s (by %s date)\n",
		r.Since.UTC().Format("2006-01-02T15:04:05Z"),
		r.Until.UTC().Format("2006-01-02T15:04:05Z"),
		basisLabel(r.WindowDateBasis))

	// An empty BaseSHA has two distinct causes, and conflating them misleads:
	// a root-commit window (BaseSource says so) genuinely has no parent to
	// compare against, whereas an empty window simply has no boundaries at all.
	// Only the former is "repository root".
	base := r.Boundaries.BaseSHA
	if base == "" {
		if r.Boundaries.BaseSource == model.BaseSourceRepositoryRoot {
			base = "(repository root)"
		} else {
			base = "(none)"
		}
	}
	head := r.Boundaries.HeadSHA
	if head == "" {
		head = "(none)"
	}
	fmt.Fprintf(b, "**Boundaries**: %s..%s", shortSHA(base), shortSHA(head))
	var parts []string
	if r.Boundaries.BaseSource != "" {
		parts = append(parts, "base: "+r.Boundaries.BaseSource)
	}
	if r.Boundaries.Status != "" {
		parts = append(parts, "status: "+r.Boundaries.Status)
	}
	if len(parts) > 0 {
		fmt.Fprintf(b, " (%s)", strings.Join(parts, ", "))
	}
	b.WriteString("\n\n")
}

func basisLabel(basis string) string {
	if basis == "" {
		return "committer"
	}
	return basis
}

func writeActivityCommits(b *strings.Builder, r model.ActivityResult) {
	fmt.Fprintf(b, "## Commits (%d)\n\n", r.Count)
	if len(r.Commits) == 0 {
		b.WriteString("_No commits in this window._\n\n")
		return
	}
	for _, c := range r.Commits {
		fmt.Fprintf(b, "- %s %s — %s",
			codeSpan(shortSHA(c.SHA)),
			c.CommitterDate.UTC().Format("2006-01-02"),
			c.Summary())
		if c.AuthorName != "" {
			fmt.Fprintf(b, " (%s)", c.AuthorName)
		}
		if c.Enriched {
			b.WriteString(" _[enriched]_")
		}
		b.WriteString("\n")
		// The full message is the evidence; the summary alone loses the "why".
		if body := commitBody(c.Message); body != "" {
			for _, line := range strings.Split(body, "\n") {
				fmt.Fprintf(b, "  > %s\n", line)
			}
		}
	}
	b.WriteString("\n")
}

// commitBody returns everything after the summary line, trimmed.
func commitBody(message string) string {
	_, rest, found := strings.Cut(message, "\n")
	if !found {
		return ""
	}
	return strings.TrimSpace(rest)
}

func writeActivityChangeSet(b *strings.Builder, r model.ActivityResult) {
	cs := r.ChangeSet
	fmt.Fprintf(b, "## Change set (%d files, +%d / -%d)",
		len(cs.Paths), cs.TotalAdditions, cs.TotalDeletions)
	if cs.Truncated {
		b.WriteString(" _(truncated by the provider)_")
	}
	b.WriteString("\n\n")

	if len(cs.Paths) == 0 {
		if r.Boundaries.Status == model.StatusDiverged {
			b.WriteString("_Suppressed: the window's boundaries do not share ancestry._\n\n")
		} else {
			b.WriteString("_No file changes between the window's boundaries._\n\n")
		}
		return
	}

	for _, p := range cs.Paths {
		path := p.Path
		if p.PreviousPath != "" {
			path = p.PreviousPath + " -> " + p.Path
		}
		fmt.Fprintf(b, "- %s", codeSpan(path))
		if p.Status != "" {
			fmt.Fprintf(b, " %s", p.Status)
		}
		fmt.Fprintf(b, " (+%d/-%d)", p.Additions, p.Deletions)
		if p.PatchTruncated && p.Patch == "" {
			b.WriteString(" _(diff truncated)_")
		}
		b.WriteString("\n")
		if p.Patch != "" {
			writeActivityPatch(b, p)
		}
	}
	b.WriteString("\n")
}

// writeActivityPatch fences patch text so untrusted commit content cannot break
// out of the code block and render as live Markdown, matching the defense in
// writeFileChange.
func writeActivityPatch(b *strings.Builder, p model.ChangedPath) {
	fence := codeFence(p.Patch)
	fmt.Fprintf(b, "\n    %sdiff\n", fence)
	indented := "    " + strings.ReplaceAll(p.Patch, "\n", "\n    ")
	b.WriteString(indented)
	if !strings.HasSuffix(p.Patch, "\n") {
		b.WriteString("\n")
	}
	if p.PatchTruncated {
		b.WriteString("    # diff truncated\n")
	}
	fmt.Fprintf(b, "    %s\n", fence)
}

// writeActivityCorrelations shows each association's basis so a reader can tell
// observation from inference at a glance. Presenting them without the basis
// would be exactly the overstatement the attribution rules exist to prevent.
func writeActivityCorrelations(b *strings.Builder, r model.ActivityResult) {
	if len(r.Correlations) == 0 {
		return
	}
	fmt.Fprintf(b, "## Correlations (%d)\n\n", len(r.Correlations))
	for _, c := range r.Correlations {
		fmt.Fprintf(b, "- %s — **%s**", codeSpan(c.Path), c.Basis)
		if c.Rule != "" {
			fmt.Fprintf(b, " (%s)", c.Rule)
		}
		if len(c.SHAs) > 0 {
			short := make([]string, 0, len(c.SHAs))
			for _, s := range c.SHAs {
				short = append(short, shortSHA(s))
			}
			fmt.Fprintf(b, ": %s", strings.Join(short, ", "))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

// writeActivityCost reports consumption on every result, including bounded
// ones: a query that stopped early still has to say what it spent.
func writeActivityCost(b *strings.Builder, cost model.CostReport) {
	b.WriteString("## Cost\n\n")

	ceiling := "no ceiling"
	if cost.Ceiling > 0 {
		ceiling = fmt.Sprintf("%d ceiling", cost.Ceiling)
	}
	fmt.Fprintf(b, "Requests: %d consumed of %s.", cost.Consumed, ceiling)
	if cost.Estimated > 0 {
		fmt.Fprintf(b, " Estimated %d.", cost.Estimated)
	}
	if cost.QuotaLimit > 0 {
		fmt.Fprintf(b, " Quota: %d of %d remaining", cost.QuotaRemaining, cost.QuotaLimit)
		if !cost.QuotaResetsAt.IsZero() {
			fmt.Fprintf(b, ", resets %s", cost.QuotaResetsAt.UTC().Format("15:04Z"))
		}
		b.WriteString(".")
	}
	b.WriteString("\n\n")
}

// writeActivityDisclosures renders disclosures as a visible section rather than
// a footnote: an agent that misses the net-comparison blind spot will overstate
// what the evidence shows.
func writeActivityDisclosures(b *strings.Builder, disclosures []model.Disclosure) {
	if len(disclosures) == 0 {
		return
	}
	b.WriteString("## Disclosures\n\n")
	for _, d := range disclosures {
		fmt.Fprintf(b, "- **%s**: %s", d.Kind, d.Reason)
		if d.NextAction != "" {
			fmt.Fprintf(b, " _Next:_ %s", d.NextAction)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func shortSHA(sha string) string {
	if len(sha) > 7 && !strings.HasPrefix(sha, "(") {
		return sha[:7]
	}
	return sha
}
