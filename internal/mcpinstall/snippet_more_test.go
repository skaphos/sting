// SPDX-License-Identifier: MIT
package mcpinstall

import (
	"strings"
	"testing"
)

func TestSnippetPerRuntime(t *testing.T) {
	e := Entry{Command: "/usr/local/bin/sting", Args: []string{"mcp"}, Enabled: true}
	cases := []struct {
		runtime string
		wantKey string // serverKey marker expected in output
	}{
		{"claude", "mcpServers"},
		{"codex", "mcp_servers"},
		{"opencode", "mcp"},
		{"grok", "mcp_servers"},
	}
	for _, c := range cases {
		t.Run(c.runtime, func(t *testing.T) {
			s, err := Snippet(c.runtime, e)
			if err != nil {
				t.Fatalf("Snippet(%s): %v", c.runtime, err)
			}
			if !strings.Contains(s, c.wantKey) {
				t.Errorf("Snippet(%s) missing %q:\n%s", c.runtime, c.wantKey, s)
			}
			if !strings.Contains(s, "sting") {
				t.Errorf("Snippet(%s) missing server key:\n%s", c.runtime, s)
			}
			if !strings.Contains(s, "/usr/local/bin/sting") {
				t.Errorf("Snippet(%s) missing command:\n%s", c.runtime, s)
			}
		})
	}
}

func TestSnippetUnknownRuntime(t *testing.T) {
	if _, err := Snippet("bogus", Entry{Command: "/bin/sting"}); err == nil {
		t.Error("Snippet(bogus) expected error")
	}
}

func TestClaudePermissionToolName(t *testing.T) {
	got := ClaudePermissionToolName("get_commits")
	if got != "mcp__sting__get_commits" {
		t.Errorf("ClaudePermissionToolName = %q", got)
	}
}

func TestClaudePermissionsSnippetSorted(t *testing.T) {
	s, err := ClaudePermissionsSnippet([]string{"zeta", "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	ai := strings.Index(s, "mcp__sting__alpha")
	zi := strings.Index(s, "mcp__sting__zeta")
	if ai < 0 || zi < 0 {
		t.Fatalf("snippet missing tools:\n%s", s)
	}
	if ai > zi {
		t.Errorf("permissions not sorted (alpha after zeta):\n%s", s)
	}
}
