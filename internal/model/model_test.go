// SPDX-License-Identifier: MIT
package model

import "testing"

func TestScopeValid(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scope Scope
		want  bool
	}{
		{"search", ScopeSearch, true},
		{"repos", ScopeRepos, true},
		{"org", ScopeOrg, true},
		{"invalid", Scope("bogus"), false},
		{"empty", Scope(""), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.scope.Valid(); got != tc.want {
				t.Errorf("Scope(%q).Valid() = %v, want %v", tc.scope, got, tc.want)
			}
		})
	}
}

func TestCommitSummary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message string
		want    string
	}{
		{"single line", "Fix window parsing", "Fix window parsing"},
		{"multi line", "Add MCP server\n\nbody text", "Add MCP server"},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Commit{Message: tc.message}
			if got := c.Summary(); got != tc.want {
				t.Errorf("Commit{Message:%q}.Summary() = %q, want %q", tc.message, got, tc.want)
			}
		})
	}
}
