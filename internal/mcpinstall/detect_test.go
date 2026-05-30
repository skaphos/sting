// SPDX-License-Identifier: MIT
package mcpinstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustDetect(t *testing.T, name string, want bool) {
	t.Helper()
	r, ok := ByName(name)
	if !ok {
		t.Fatalf("adapter %q not registered", name)
	}
	got, err := r.Detect()
	if err != nil {
		t.Fatalf("%s Detect: %v", name, err)
	}
	if got != want {
		t.Errorf("%s Detect = %v, want %v", name, got, want)
	}
}

func TestClaudeDetect(t *testing.T) {
	home := isolateHome(t)
	mustDetect(t, "claude", false)
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustDetect(t, "claude", true)
}

func TestClaudeDetectDir(t *testing.T) {
	home := isolateHome(t)
	mustDetect(t, "claude", false)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustDetect(t, "claude", true)
}

func TestCodexDetect(t *testing.T) {
	home := isolateHome(t)
	mustDetect(t, "codex", false)
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustDetect(t, "codex", true)
}

func TestGrokDetectHomeDir(t *testing.T) {
	home := isolateHome(t)
	mustDetect(t, "grok", false)
	if err := os.MkdirAll(filepath.Join(home, ".grok"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustDetect(t, "grok", true)
}

func TestGrokDetectEnvOverride(t *testing.T) {
	isolateHome(t)
	mustDetect(t, "grok", false)
	t.Setenv("GROK_CONFIG_DIR", t.TempDir())
	mustDetect(t, "grok", true)
}

func TestGrokDetectConfigFile(t *testing.T) {
	home := isolateHome(t)
	dir := filepath.Join(home, ".grok")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	mustDetect(t, "grok", true)
}

func TestOpencodeDetect(t *testing.T) {
	home := isolateHome(t)
	mustDetect(t, "opencode", false)
	if err := os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustDetect(t, "opencode", true)
}

func TestOpencodeDetectEnvOverride(t *testing.T) {
	isolateHome(t)
	mustDetect(t, "opencode", false)
	t.Setenv("OPENCODE_CONFIG_DIR", t.TempDir())
	mustDetect(t, "opencode", true)
}

func TestOpencodeDetectXDG(t *testing.T) {
	isolateHome(t)
	mustDetect(t, "opencode", false)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	mustDetect(t, "opencode", false)
	if err := os.MkdirAll(filepath.Join(xdg, "opencode"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustDetect(t, "opencode", true)
}

func TestConfigPathsUserScope(t *testing.T) {
	home := isolateHome(t)
	cases := []struct {
		name string
		want string
	}{
		{"claude", filepath.Join(home, ".claude.json")},
		{"codex", filepath.Join(home, ".codex", "config.toml")},
		{"grok", filepath.Join(home, ".grok", "config.toml")},
		{"opencode", filepath.Join(home, ".config", "opencode", "opencode.json")},
	}
	for _, c := range cases {
		r, _ := ByName(c.name)
		got, err := r.ConfigPath(ScopeUser)
		if err != nil {
			t.Fatalf("%s ConfigPath(user): %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s user path = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestConfigPathsProjectScope(t *testing.T) {
	isolateHome(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		want string
	}{
		{"claude", filepath.Join(cwd, ".mcp.json")},
		{"grok", filepath.Join(cwd, ".grok", "config.toml")},
		{"opencode", filepath.Join(cwd, "opencode.json")},
	}
	for _, c := range cases {
		r, _ := ByName(c.name)
		got, err := r.ConfigPath(ScopeProject)
		if err != nil {
			t.Fatalf("%s ConfigPath(project): %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s project path = %q, want %q", c.name, got, c.want)
		}
	}

	// codex has no project scope.
	r, _ := ByName("codex")
	if _, err := r.ConfigPath(ScopeProject); err != ErrScopeUnsupported {
		t.Errorf("codex project = %v, want ErrScopeUnsupported", err)
	}
}

func TestConfigPathsUnknownScope(t *testing.T) {
	isolateHome(t)
	for _, name := range []string{"claude", "codex", "grok", "opencode"} {
		r, _ := ByName(name)
		if _, err := r.ConfigPath(Scope(99)); err == nil {
			t.Errorf("%s ConfigPath(unknown) returned nil error", name)
		} else if !strings.Contains(err.Error(), "unknown scope") {
			t.Errorf("%s ConfigPath(unknown) err = %v", name, err)
		}
	}
}

func TestConfigPathGrokEnvOverride(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	t.Setenv("GROK_CONFIG_DIR", dir)
	r, _ := ByName("grok")
	got, err := r.ConfigPath(ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, "config.toml") {
		t.Errorf("grok user path with override = %q", got)
	}
}

func TestConfigPathOpencodeEnvOverride(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	t.Setenv("OPENCODE_CONFIG_DIR", dir)
	r, _ := ByName("opencode")
	got, err := r.ConfigPath(ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, "opencode.json") {
		t.Errorf("opencode user path with override = %q", got)
	}
}

func TestConfigPathOpencodeXDG(t *testing.T) {
	isolateHome(t)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	r, _ := ByName("opencode")
	got, err := r.ConfigPath(ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(xdg, "opencode", "opencode.json") {
		t.Errorf("opencode user path with XDG = %q", got)
	}
}
