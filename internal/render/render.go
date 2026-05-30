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
	fmt.Fprintf(&b, "# Commits by `%s`\n\n", r.Author)
	fmt.Fprintf(&b, "- **Window:** %s → %s\n",
		r.Since.UTC().Format("2006-01-02"), r.Until.UTC().Format("2006-01-02"))
	fmt.Fprintf(&b, "- **Scope:** %s\n", r.Scope)
	fmt.Fprintf(&b, "- **Commits:** %d", r.Count)
	if r.Truncated {
		b.WriteString(" _(truncated)_")
	}
	b.WriteString("\n\n")

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
			fmt.Fprintf(&b, "- `%s` %s — %s",
				sha, c.Date.UTC().Format("2006-01-02"), c.Summary())
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

func writeFileChange(b *strings.Builder, f model.File) {
	path := f.Path
	if f.PreviousPath != "" {
		path = f.PreviousPath + " -> " + f.Path
	}
	fmt.Fprintf(b, "  - `%s`", path)
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
		b.WriteString("\n```diff\n")
		b.WriteString(f.Patch)
		if !strings.HasSuffix(f.Patch, "\n") {
			b.WriteString("\n")
		}
		if f.PatchTruncated {
			b.WriteString("\n# diff truncated\n")
		}
		b.WriteString("```\n")
	}
}
