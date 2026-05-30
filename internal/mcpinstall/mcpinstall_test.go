// SPDX-License-Identifier: MIT
package mcpinstall

import (
	"os"
	"strings"
	"testing"
)

// isolateHome points every runtime's user-scope config under a fresh temp dir
// so tests never touch the real environment.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	// os.UserHomeDir reads %USERPROFILE% on Windows, so isolate it too.
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GROK_CONFIG_DIR", "")
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	return home
}

func TestAdapterRoundTrip(t *testing.T) {
	for _, name := range []string{"claude", "codex", "grok", "opencode"} {
		t.Run(name, func(t *testing.T) {
			isolateHome(t)
			r, ok := ByName(name)
			if !ok {
				t.Fatalf("adapter %q not registered", name)
			}
			path, err := r.ConfigPath(ScopeUser)
			if err != nil {
				t.Fatalf("ConfigPath: %v", err)
			}

			want := Entry{Command: "/usr/local/bin/sting", Args: []string{"mcp"}, Enabled: true}
			if err := r.WriteEntry(path, want); err != nil {
				t.Fatalf("WriteEntry: %v", err)
			}

			got, present, err := r.ReadEntry(path)
			if err != nil {
				t.Fatalf("ReadEntry: %v", err)
			}
			if !present {
				t.Fatal("entry not present after write")
			}
			if got.Command != want.Command {
				t.Errorf("command = %q, want %q", got.Command, want.Command)
			}
			if strings.Join(got.Args, ",") != "mcp" {
				t.Errorf("args = %v, want [mcp]", got.Args)
			}
			if !got.Enabled {
				t.Error("enabled = false, want true")
			}

			removed, err := r.RemoveEntry(path)
			if err != nil {
				t.Fatalf("RemoveEntry: %v", err)
			}
			if !removed {
				t.Error("RemoveEntry reported nothing removed")
			}
			_, present, err = r.ReadEntry(path)
			if err != nil {
				t.Fatalf("ReadEntry after remove: %v", err)
			}
			if present {
				t.Error("entry still present after remove")
			}
		})
	}
}

// TestWritePreservesOtherKeys ensures registering sting does not clobber an
// existing, unrelated server entry in the same config file.
func TestWritePreservesOtherKeys(t *testing.T) {
	isolateHome(t)
	r, _ := ByName("claude")
	path, err := r.ConfigPath(ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	seed := `{"mcpServers":{"other":{"command":"/bin/other"}},"someTopLevel":true}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.WriteEntry(path, Entry{Command: "/bin/sting", Args: []string{"mcp"}, Enabled: true}); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, want := range []string{`"other"`, `/bin/other`, `"someTopLevel"`, `"sting"`, `/bin/sting`} {
		if !strings.Contains(out, want) {
			t.Errorf("config missing %q after write:\n%s", want, out)
		}
	}
}

func TestRemoveAbsentIsNoOp(t *testing.T) {
	isolateHome(t)
	r, _ := ByName("codex")
	path, _ := r.ConfigPath(ScopeUser)
	removed, err := r.RemoveEntry(path)
	if err != nil {
		t.Fatalf("RemoveEntry on missing file: %v", err)
	}
	if removed {
		t.Error("RemoveEntry reported removal from a nonexistent config")
	}
}

func TestCodexProjectScopeUnsupported(t *testing.T) {
	r, _ := ByName("codex")
	if _, err := r.ConfigPath(ScopeProject); err != ErrScopeUnsupported {
		t.Errorf("codex project scope err = %v, want ErrScopeUnsupported", err)
	}
}

func TestSnippetsContainServerKey(t *testing.T) {
	e := Entry{Command: "/bin/sting", Args: []string{"mcp"}, Enabled: true}
	for _, name := range manualRuntimes() {
		s, err := Snippet(name, e)
		if err != nil {
			t.Fatalf("Snippet(%s): %v", name, err)
		}
		if !strings.Contains(s, "sting") || !strings.Contains(s, "/bin/sting") {
			t.Errorf("Snippet(%s) missing server key or command:\n%s", name, s)
		}
	}
}

func TestClaudePermissionsSnippet(t *testing.T) {
	s, err := ClaudePermissionsSnippet([]string{"get_commits"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "mcp__sting__get_commits") {
		t.Errorf("permissions snippet missing tool identifier:\n%s", s)
	}
}

func manualRuntimes() []string { return []string{"claude", "codex", "opencode", "grok"} }
