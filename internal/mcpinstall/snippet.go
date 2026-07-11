// SPDX-License-Identifier: MIT
package mcpinstall

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Snippet returns a self-contained config fragment that registers sting under
// the named runtime, matching what the adapter would write. Intended for
// `sting install --manual`, so a user can paste it verbatim.
func Snippet(runtime string, e Entry) (string, error) {
	switch runtime {
	case "claude":
		return renderJSON(map[string]any{
			"mcpServers": map[string]any{
				serverKey: claudeServer{Command: e.Command, Args: e.Args},
			},
		})
	case "codex":
		return renderTOML(map[string]any{
			"mcp_servers": map[string]any{
				serverKey: codexServer{Command: e.Command, Args: e.Args},
			},
		})
	case "opencode":
		argv := append([]string{e.Command}, e.Args...)
		return renderJSON(map[string]any{
			"mcp": map[string]any{
				serverKey: opencodeServer{Type: "local", Command: argv, Enabled: e.Enabled},
			},
		})
	case "grok":
		return renderTOML(map[string]any{
			"mcp_servers": map[string]any{
				serverKey: grokServer(e),
			},
		})
	default:
		return "", fmt.Errorf("unknown runtime: %q", runtime)
	}
}

// ClaudePermissionToolName returns the identifier Claude Code matches in
// permissions.allow for a sting MCP tool: mcp__<server>__<tool>.
func ClaudePermissionToolName(tool string) string {
	return "mcp__" + serverKey + "__" + tool
}

// ClaudePermissionsSnippet renders a ~/.claude/settings.json fragment that
// allow-lists the given tools under permissions.allow. sting's tools are all
// read-only, so auto-approving them is safe.
func ClaudePermissionsSnippet(toolNames []string) (string, error) {
	allow := make([]string, 0, len(toolNames))
	for _, t := range toolNames {
		allow = append(allow, ClaudePermissionToolName(t))
	}
	sort.Strings(allow)
	return renderJSON(map[string]any{
		"permissions": map[string]any{"allow": allow},
	})
}

func renderJSON(doc map[string]any) (string, error) {
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(raw), "\n") + "\n", nil
}

func renderTOML(doc map[string]any) (string, error) {
	raw, err := toml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(raw), "\n") + "\n", nil
}
