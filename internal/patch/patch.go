// SPDX-License-Identifier: MIT
// Package patch provides helpers for handling bounded commit patch/diff text.
package patch

// ConsumePatchBudget truncates patch text to a remaining byte budget.
// It returns the (possibly truncated) patch, a flag indicating whether
// truncation occurred, and the budget remaining after consumption.
//
// This is used by both the GitHub and GitLab clients to enforce
// max_diff_bytes uniformly.
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
	return patch[:budget], true, 0
}
