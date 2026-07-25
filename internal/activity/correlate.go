// SPDX-License-Identifier: MIT
package activity

import (
	"sort"
	"strings"

	"github.com/skaphos/sting/model"
)

// Correlate links each changed path to the commits that plausibly produced it,
// applying three declared rules in a fixed order with first match winning.
//
// The ordering is deliberate and the labels are load-bearing:
//
//  1. observed — per-commit file data was actually fetched and lists the path.
//     Certain.
//  2. inferred:path-mention — the commit message body contains the path
//     verbatim. Strong.
//  3. inferred:scope-match — a Conventional Commit scope matches a leading path
//     segment. Weak: a scope names a component, not a path, and the two only
//     usually coincide.
//
// A path that matches no rule is left out entirely rather than guessed at. An
// honest gap is worth more than a fabricated link, and presenting inference as
// observation is precisely what the basis label exists to prevent.
//
// Nothing here depends on map iteration order, so repeated runs over identical
// input produce byte-identical output.
func Correlate(paths []model.ChangedPath, commits []model.ActivityCommit) []model.Correlation {
	if len(paths) == 0 || len(commits) == 0 {
		return nil
	}

	out := make([]model.Correlation, 0, len(paths))
	for _, p := range paths {
		if c, ok := correlatePath(p.Path, commits); ok {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return nil
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// correlatePath applies the rules to one path, returning the first match.
func correlatePath(path string, commits []model.ActivityCommit) (model.Correlation, bool) {
	// Rule 1: observed. Only commits whose per-commit detail was actually
	// fetched may be named here — this is the invariant behind the whole
	// attribution story.
	if shas := observedSHAs(path, commits); len(shas) > 0 {
		return model.Correlation{Path: path, SHAs: shas, Basis: model.BasisObserved}, true
	}

	// Rule 2: the message body names the path verbatim.
	if shas := matchingSHAs(commits, func(c model.ActivityCommit) bool {
		return strings.Contains(c.Message, path)
	}); len(shas) > 0 {
		return model.Correlation{
			Path: path, SHAs: shas,
			Basis: model.BasisInferred, Rule: model.RulePathMention,
		}, true
	}

	// Rule 3: a Conventional Commit scope matches a leading path segment.
	if segment := leadingSegment(path); segment != "" {
		if shas := matchingSHAs(commits, func(c model.ActivityCommit) bool {
			scope, ok := conventionalScope(c.Summary())
			return ok && scope == segment
		}); len(shas) > 0 {
			return model.Correlation{
				Path: path, SHAs: shas,
				Basis: model.BasisInferred, Rule: model.RuleScopeMatch,
			}, true
		}
	}

	return model.Correlation{}, false
}

// observedSHAs returns the enriched commits whose fetched file list contains
// path. A commit that was never enriched can never appear here, which is what
// keeps "observed" honest.
func observedSHAs(path string, commits []model.ActivityCommit) []string {
	return matchingSHAs(commits, func(c model.ActivityCommit) bool {
		if !c.Enriched {
			return false
		}
		for _, f := range c.Files {
			if f.Path == path || (f.PreviousPath != "" && f.PreviousPath == path) {
				return true
			}
		}
		return false
	})
}

// matchingSHAs collects the SHAs of commits satisfying pred, in the commits'
// existing order, then sorts them so the output does not depend on how the
// provider happened to order the window.
func matchingSHAs(commits []model.ActivityCommit, pred func(model.ActivityCommit) bool) []string {
	var shas []string
	for _, c := range commits {
		if c.SHA != "" && pred(c) {
			shas = append(shas, c.SHA)
		}
	}
	if len(shas) == 0 {
		return nil
	}
	sort.Strings(shas)
	return shas
}

// leadingSegment returns the first path segment, which is what a Conventional
// Commit scope is compared against. A top-level file has no leading directory,
// so it yields nothing and the scope rule cannot fire for it.
func leadingSegment(path string) string {
	segment, _, ok := strings.Cut(path, "/")
	if !ok {
		return ""
	}
	return segment
}

// conventionalScope extracts the scope from a Conventional Commit summary —
// the "render" in "feat(render): add activity view".
func conventionalScope(summary string) (string, bool) {
	open := strings.Index(summary, "(")
	if open <= 0 {
		return "", false
	}
	rest := summary[open+1:]
	close := strings.Index(rest, ")")
	if close <= 0 {
		return "", false
	}
	// The scope must be part of the Conventional Commit prefix, so a colon has
	// to follow the closing parenthesis; otherwise a parenthetical anywhere in
	// the summary would be read as a scope.
	after := strings.TrimSpace(rest[close+1:])
	if !strings.HasPrefix(after, ":") && !strings.HasPrefix(after, "!:") {
		return "", false
	}
	scope := strings.TrimSpace(rest[:close])
	if scope == "" {
		return "", false
	}
	return scope, true
}
