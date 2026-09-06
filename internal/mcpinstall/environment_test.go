// SPDX-License-Identifier: MIT
package mcpinstall

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func readEnvironmentEntry(t *testing.T, runtime, raw string) map[string]any {
	t.Helper()
	var doc map[string]any
	var err error
	key := "mcp_servers"
	if runtime == "claude" || runtime == "opencode" {
		err = json.Unmarshal([]byte(raw), &doc)
		key = "mcpServers"
		if runtime == "opencode" {
			key = "mcp"
		}
	} else {
		err = toml.Unmarshal([]byte(raw), &doc)
	}
	if err != nil {
		t.Fatal(err)
	}
	return doc[key].(map[string]any)["sting"].(map[string]any)
}

func TestCredentialForwardingDefaults(t *testing.T) {
	// An installer must record references, never snapshot these values.
	t.Setenv("STING_TOKEN", "secret-github-install-sentinel")
	t.Setenv("STING_GITLAB_TOKEN", "secret-gitlab-install-sentinel")
	for _, runtime := range manualRuntimes() {
		t.Run(runtime, func(t *testing.T) {
			r, _ := ByName(runtime)
			path := filepath.Join(t.TempDir(), "config")
			e := Entry{Command: "/bin/sting", Args: []string{"mcp"}, Enabled: true}
			if err := r.WriteEntry(path, e); err != nil {
				t.Fatal(err)
			}
			raw := readFile(t, path)
			snippet, err := Snippet(runtime, e)
			if err != nil {
				t.Fatal(err)
			}
			entry := readEnvironmentEntry(t, runtime, raw)
			if !reflect.DeepEqual(entry, readEnvironmentEntry(t, runtime, snippet)) {
				t.Fatal("manual snippet differs from installed entry")
			}
			if runtime == "codex" {
				if !reflect.DeepEqual(entry["env_vars"], []any{"STING_TOKEN", "STING_GITLAB_TOKEN"}) {
					t.Errorf("missing Codex credential forwarding: %v", entry["env_vars"])
				}
			} else {
				field, github, gitlab := "env", "${STING_TOKEN:-}", "${STING_GITLAB_TOKEN:-}"
				if runtime == "opencode" {
					field, github, gitlab = "environment", "{env:STING_TOKEN}", "{env:STING_GITLAB_TOKEN}"
				}
				want := map[string]any{"STING_TOKEN": github, "STING_GITLAB_TOKEN": gitlab}
				if !reflect.DeepEqual(entry[field], want) {
					t.Errorf("credential references = %v, want %v", entry[field], want)
				}
			}
			for _, output := range []string{raw, snippet} {
				if strings.Contains(output, "install-sentinel") {
					t.Fatal("installer copied a credential value")
				}
			}
			if err := r.WriteEntry(path, e); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, path); got != raw {
				t.Fatal("reinstall is not idempotent")
			}
			// OAuth-only installation produces identical references even when
			// neither credential is available to the installer.
			t.Setenv("STING_TOKEN", "")
			t.Setenv("STING_GITLAB_TOKEN", "")
			fresh := filepath.Join(t.TempDir(), "config")
			if err := r.WriteEntry(fresh, e); err != nil {
				t.Fatal(err)
			}
			if readFile(t, fresh) != raw {
				t.Fatal("installation depends on credential availability")
			}
		})
	}
}

func TestCredentialForwardingPreservesOverrides(t *testing.T) {
	seeds := map[string]string{
		"claude":   `{"mcpServers":{"sting":{"command":"old","env":{"STING_TOKEN":"explicit","CUSTOM":"keep"}}}}`,
		"opencode": `{"mcp":{"sting":{"command":["old"],"environment":{"STING_TOKEN":"explicit","CUSTOM":"keep"}}}}`,
		"grok":     "[mcp_servers.sting]\ncommand = 'old'\nenv = {STING_TOKEN = 'explicit', CUSTOM = 'keep'}\n",
		"codex":    "[mcp_servers.sting]\ncommand = 'old'\nenv_vars = ['CUSTOM', {name = 'STING_GITLAB_TOKEN', source = 'remote'}]\nenv = {STING_TOKEN = 'explicit'}\n",
	}
	for runtime, seed := range seeds {
		t.Run(runtime, func(t *testing.T) {
			path := writeTemp(t, "config", seed)
			r, _ := ByName(runtime)
			e := Entry{Command: "new", Args: []string{"mcp"}, Enabled: true}
			if err := r.WriteEntry(path, e); err != nil {
				t.Fatal(err)
			}
			raw := readFile(t, path)
			entry := readEnvironmentEntry(t, runtime, raw)
			field := "env"
			if runtime == "opencode" {
				field = "environment"
			}
			env := entry[field].(map[string]any)
			if env["STING_TOKEN"] != "explicit" {
				t.Fatal("explicit credential was overwritten")
			}
			if runtime == "codex" {
				original := readEnvironmentEntry(t, runtime, seed)
				if !reflect.DeepEqual(entry["env_vars"], original["env_vars"]) {
					t.Fatal("existing forwarding sources were replaced or shadowed")
				}
			} else if env["CUSTOM"] != "keep" || env["STING_GITLAB_TOKEN"] == nil {
				t.Fatal("existing environment lost or missing credential not filled")
			}
			if err := r.WriteEntry(path, e); err != nil {
				t.Fatal(err)
			}
			if readFile(t, path) != raw {
				t.Fatal("reinstall changed preserved settings")
			}
			if _, present, err := r.ReadEntry(path); err != nil || !present {
				t.Fatalf("installed entry cannot be read: present=%v err=%v", present, err)
			}
		})
	}
}

func TestCodexCredentialForwardingExtendsExistingList(t *testing.T) {
	path := writeTemp(t, "config.toml", "[mcp_servers.sting]\ncommand = 'old'\nenv_vars = ['CUSTOM', 'STING_TOKEN']\n")
	r, _ := ByName("codex")
	if err := r.WriteEntry(path, Entry{Command: "new"}); err != nil {
		t.Fatal(err)
	}
	entry := readEnvironmentEntry(t, "codex", readFile(t, path))
	want := []any{"CUSTOM", "STING_TOKEN", "STING_GITLAB_TOKEN"}
	if !reflect.DeepEqual(entry["env_vars"], want) {
		t.Errorf("env_vars = %v, want %v", entry["env_vars"], want)
	}
}

func TestCredentialForwardingRejectsMalformedSettings(t *testing.T) {
	cases := []struct{ runtime, seed string }{
		{"claude", `{"mcpServers":{"sting":{"command":"old","env":"invalid"}}}`},
		{"opencode", `{"mcp":{"sting":{"command":["old"],"environment":42}}}`},
		{"grok", "[mcp_servers.sting]\ncommand = 'old'\nenv = 'invalid'\n"},
		{"codex", "[mcp_servers.sting]\ncommand = 'old'\nenv_vars = 'invalid'\n"},
		{"codex", "[mcp_servers.sting]\ncommand = 'old'\nenv_vars = [42]\n"},
		{"codex", "[mcp_servers.sting]\ncommand = 'old'\nenv_vars = [{source = 'remote'}]\n"},
	}
	for _, tc := range cases {
		t.Run(tc.runtime, func(t *testing.T) {
			path := writeTemp(t, "config", tc.seed)
			r, _ := ByName(tc.runtime)
			if err := r.WriteEntry(path, Entry{Command: "new"}); err == nil {
				t.Fatal("malformed environment configuration accepted")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != tc.seed {
				t.Fatal("rejected configuration was changed")
			}
		})
	}
}
