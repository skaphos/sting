// SPDX-License-Identifier: MIT
// Package patch provides helpers for handling bounded commit patch/diff text.
package patch

import "unicode/utf8"

// ConsumePatchBudget truncates patch text to a remaining byte budget.
// It returns the (possibly truncated) patch, a flag indicating whether
// truncation occurred, and the budget remaining after consumption.
//
// This is used by both the GitHub and GitLab clients to enforce
// max_diff_bytes uniformly.
//
// When truncation is required it backs off to the last whole-rune boundary at
// or below the byte budget so recorded evidence is always valid UTF-8 and never
// ends in a half-written multibyte character.
func ConsumePatchBudget(patch string, budget int) (string, bool, int) {
	if patch == "" {
		return "", false, budget
	}
	if budget <= 0 {
		return "", true, 0
	}
	if len(patch) <= budget {
		return patch, false, budget - len(patch)
	}
	end := budget
	for end > 0 && !utf8.RuneStart(patch[end]) {
		end--
	}
	return patch[:end], true, 0
}
