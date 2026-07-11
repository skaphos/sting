// SPDX-License-Identifier: MIT
// Package render serializes a query Result into the supported output formats.
package render

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/skaphos/sting/model"
)

// Format identifies an output encoding.
type Format string

const (
	FormatJSON     Format = "json"
	FormatMarkdown Format = "markdown"
)

// Parse normalizes a user-supplied format string, defaulting to Markdown.
func Parse(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "markdown", "md":
		return FormatMarkdown, nil
	case "json":
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("unknown format %q (want markdown|json)", s)
	}
}

// Render encodes the result in the requested format.
func Render(r model.Result, f Format) (string, error) {
	switch f {
	case FormatJSON:
		return toJSON(r)
	case FormatMarkdown:
		return toMarkdown(r), nil
	default:
		return "", fmt.Errorf("unknown format %q", f)
	}
}

// Markdown renders r as Markdown. It never fails, so it is convenient for
// callers that always want the human-readable form.
func Markdown(r model.Result) string {
	return toMarkdown(r)
}

func toJSON(r model.Result) (string, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode json: %w", err)
	}
	return string(b), nil
}

func toMarkdown(r model.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Commits by %s\n\n", codeSpan(r.Author))
	fmt.Fprintf(&b, "- **Window:** %s → %s\n",
		r.Since.UTC().Format("2006-01-02"), r.Until.UTC().Format("2006-01-02"))
	fmt.Fprintf(&b, "- **Scope:** %s\n", r.Scope)
	fmt.Fprintf(&b, "- **Commits:** %d", r.Count)
	if r.Truncated {
		b.WriteString(" _(truncated)_")
	}
	b.WriteString("\n")
	writeSkipped(&b, r.Skipped)
	b.WriteString("\n")

	if r.Count == 0 {
		b.WriteString("_No commits found in this window._\n")
		return b.String()
	}

	// Group by repository, repos and commits each newest-first.
	byRepo := map[string][]model.Commit{}
	for _, c := range r.Commits {
		byRepo[c.Repo] = append(byRepo[c.Repo], c)
	}
	repos := make([]string, 0, len(byRepo))
	for repo := range byRepo {
		repos = append(repos, repo)
	}
	sort.Strings(repos)

	for _, repo := range repos {
		commits := byRepo[repo]
		sort.SliceStable(commits, func(i, j int) bool {
			return commits[i].Date.After(commits[j].Date)
		})
		fmt.Fprintf(&b, "## %s\n\n", repo)
		for _, c := range commits {
			sha := c.SHA
			if len(sha) > 7 {
				sha = sha[:7]
			}
			fmt.Fprintf(&b, "- %s %s — %s",
				codeSpan(sha), c.Date.UTC().Format("2006-01-02"), c.Summary())
			// Flag commits discovered on an open PR branch: they are unmerged
			// evidence, so the source matters for an auditor reading the report.
			if strings.HasPrefix(c.Source, "pull/") {
				fmt.Fprintf(&b, " [%s]", c.Source)
			}
			if c.Additions != 0 || c.Deletions != 0 {
				fmt.Fprintf(&b, " (+%d/-%d", c.Additions, c.Deletions)
				if c.Changes != 0 {
					fmt.Fprintf(&b, ", %d lines", c.Changes)
				}
				b.WriteString(")")
			}
			b.WriteString("\n")
			for _, f := range c.Files {
				writeFileChange(&b, f)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// writeSkipped notes any repositories an org scan skipped, so a partial result
// is visibly partial rather than silently short. It writes nothing when none
// were skipped.
func writeSkipped(b *strings.Builder, skipped []model.SkippedRepo) {
	if len(skipped) == 0 {
		return
	}
	fmt.Fprintf(b, "- **Skipped:** %d repo(s) could not be read\n", len(skipped))
	sorted := make([]model.SkippedRepo, len(skipped))
	copy(sorted, skipped)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Repo < sorted[j].Repo })
	for _, s := range sorted {
		fmt.Fprintf(b, "  - %s — %s\n", codeSpan(s.Repo), s.Reason)
	}
}

func writeFileChange(b *strings.Builder, f model.File) {
	path := f.Path
	if f.PreviousPath != "" {
		path = f.PreviousPath + " -> " + f.Path
	}
	fmt.Fprintf(b, "  - %s", codeSpan(path))
	if f.Status != "" {
		fmt.Fprintf(b, " %s", f.Status)
	}
	if f.Additions != 0 || f.Deletions != 0 {
		fmt.Fprintf(b, " (+%d/-%d", f.Additions, f.Deletions)
		if f.Changes != 0 {
			fmt.Fprintf(b, ", %d lines", f.Changes)
		}
		b.WriteString(")")
	}
	if f.PatchTruncated && f.Patch == "" {
		b.WriteString(" _(diff truncated)_")
	}
	b.WriteString("\n")
	if f.Patch != "" {
		// The patch is untrusted commit content fed to an LLM agent as
		// evidence, so it must not be able to break out of this fenced code
		// block: a fence marker of exactly 3 backticks embedded in the patch
		// (which, after indenting, would line up with our own fence) would
		// otherwise close the block early and let the rest of the patch
		// render as live Markdown. Use a fence longer than the longest
		// backtick run actually present in the patch so no line inside it
		// can ever match or exceed the fence length.
		fence := codeFence(f.Patch)
		fmt.Fprintf(b, "\n    %sdiff\n", fence)
		indented := "    " + strings.ReplaceAll(f.Patch, "\n", "\n    ")
		b.WriteString(indented)
		if !strings.HasSuffix(f.Patch, "\n") {
			b.WriteString("\n")
		}
		if f.PatchTruncated {
			b.WriteString("    # diff truncated\n")
		}
		fmt.Fprintf(b, "    %s\n", fence)
	}
}

// codeSpan wraps s as a CommonMark inline code span, safe against s
// containing backticks: the delimiter uses one more backtick than the
// longest backtick run in s, and a padding space is added when s itself
// starts or ends with a backtick so the delimiter can't fuse with it.
func codeSpan(s string) string {
	fence := strings.Repeat("`", longestBacktickRun(s)+1)
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		return fence + " " + s + " " + fence
	}
	return fence + s + fence
}

// codeFence returns a fenced-code-block delimiter (backticks only, no
// language tag) long enough that no run of backticks inside body can match
// or exceed it, so body can never prematurely close the block it is about to
// be wrapped in.
func codeFence(body string) string {
	n := longestBacktickRun(body) + 1
	if n < 3 {
		n = 3
	}
	return strings.Repeat("`", n)
}

// longestBacktickRun returns the length of the longest run of consecutive
// backtick characters in s.
func longestBacktickRun(s string) int {
	longest, cur := 0, 0
	for _, r := range s {
		if r == '`' {
			cur++
			if cur > longest {
				longest = cur
			}
		} else {
			cur = 0
		}
	}
	return longest
}
