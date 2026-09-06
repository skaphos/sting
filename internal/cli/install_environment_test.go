// SPDX-License-Identifier: MIT
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skaphos/sting/internal/mcpinstall"
)

func TestRunInstallUpgradesCredentialForwarding(t *testing.T) {
	seeds := map[string]string{
		"claude":   `{"mcpServers":{"sting":{"command":"/tmp/x","args":["mcp"]}}}`,
		"opencode": `{"mcp":{"sting":{"type":"local","command":["/tmp/x","mcp"],"enabled":false}}}`,
		"codex":    "[mcp_servers.sting]\ncommand = '/tmp/x'\nargs = ['mcp']\n",
		"grok":     "[mcp_servers.sting]\ncommand = '/tmp/x'\nargs = ['mcp']\nenabled = false\n",
	}
	for runtime, seed := range seeds {
		t.Run(runtime, func(t *testing.T) {
			isolateHome(t)
			r, _ := mcpinstall.ByName(runtime)
			path, err := r.ConfigPath(mcpinstall.ScopeUser)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
				t.Fatal(err)
			}
			before, _, err := r.ReadEntry(path)
			if err != nil {
				t.Fatal(err)
			}
			run := func() string {
				cmd, out, _ := newCmd()
				registerInstallFlags(cmd)
				for key, value := range map[string]string{runtime: "true", "command": "/tmp/x"} {
					if err := cmd.Flags().Set(key, value); err != nil {
						t.Fatal(err)
					}
				}
				if err := runInstall(cmd, nil); err != nil {
					t.Fatal(err)
				}
				return out.String()
			}
			if output := run(); !strings.Contains(output, "updated "+runtime) {
				t.Fatalf("old entry was not upgraded: %s", output)
			}
			after, present, err := r.ReadEntry(path)
			if err != nil || !present || !after.CredentialEnvConfigured {
				t.Fatalf("credential forwarding missing: present=%v err=%v", present, err)
			}
			if after.Enabled != before.Enabled {
				t.Fatal("install changed the user's enabled setting")
			}
			if output := run(); !strings.Contains(output, "unchanged "+runtime) {
				t.Fatalf("already configured entry was rewritten: %s", output)
			}
		})
	}
}
