// SPDX-License-Identifier: MIT
package mcpinstall

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestClaudeLargeIntegerRoundTrip is the P0 regression: a large integer anywhere
// in ~/.claude.json must survive a sting install, which rewrites the whole file.
// Before decoding with UseNumber, json.Unmarshal coerced it to float64 and
// MarshalIndent wrote back the rounded value, silently corrupting it.
func TestClaudeLargeIntegerRoundTrip(t *testing.T) {
	// 9007199254740993 = 2^53 + 1, the first integer float64 cannot represent.
	const big = "9007199254740993"
	seed := `{"numUserMessages": ` + big + `, "mcpServers": {"other": {"command": "/x"}}}`
	path := writeTemp(t, ".claude.json", seed)
	r, _ := ByName("claude")
	if err := r.WriteEntry(path, Entry{Command: "/bin/sting", Args: []string{"mcp"}, Enabled: true}); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}
	got := readFile(t, path)
	if !strings.Contains(got, big) {
		t.Errorf("large integer corrupted; want %s intact:\n%s", big, got)
	}
	if strings.Contains(got, "9007199254740992") || strings.Contains(got, "e+") {
		t.Errorf("large integer was rounded/reformatted:\n%s", got)
	}
}

// TestClaudeNullMcpServers ensures an explicit "mcpServers": null is treated as
// absent rather than a parse error, and a subsequent write succeeds.
func TestClaudeNullMcpServers(t *testing.T) {
	path := writeTemp(t, ".claude.json", `{"mcpServers": null}`)
	r, _ := ByName("claude")
	if _, present, err := r.ReadEntry(path); err != nil {
		t.Fatalf("ReadEntry with null mcpServers: %v", err)
	} else if present {
		t.Error("present = true for null mcpServers")
	}
	if err := r.WriteEntry(path, Entry{Command: "/bin/sting", Args: []string{"mcp"}, Enabled: true}); err != nil {
		t.Fatalf("WriteEntry after null: %v", err)
	}
	if _, present, err := r.ReadEntry(path); err != nil || !present {
		t.Fatalf("ReadEntry after write: present=%v err=%v", present, err)
	}
}

// TestClaudeWritePreservesUserKeys is the P1 regression: a sting entry carrying
// user-added keys (env with a token) must keep them when the command path
// changes on upgrade.
func TestClaudeWritePreservesUserKeys(t *testing.T) {
	seed := `{"mcpServers":{"sting":{"command":"/old/sting","args":["mcp"],"env":{"TOKEN":"secret"},"timeout":30}}}`
	path := writeTemp(t, ".claude.json", seed)
	r, _ := ByName("claude")
	if err := r.WriteEntry(path, Entry{Command: "/new/sting", Args: []string{"mcp"}, Enabled: true}); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}
	got := readFile(t, path)
	for _, want := range []string{"TOKEN", "secret", "timeout", "/new/sting"} {
		if !strings.Contains(got, want) {
			t.Errorf("upgrade dropped %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "/old/sting") {
		t.Errorf("stale command not replaced:\n%s", got)
	}
}

// TestOpencodeWritePreservesUserKeys is the P1 regression for OpenCode.
func TestOpencodeWritePreservesUserKeys(t *testing.T) {
	seed := `{"mcp":{"sting":{"type":"local","command":["/old/sting","mcp"],"enabled":true,"environment":{"TOKEN":"secret"}}}}`
	path := writeTemp(t, "opencode.json", seed)
	r, _ := ByName("opencode")
	if err := r.WriteEntry(path, Entry{Command: "/new/sting", Args: []string{"mcp"}, Enabled: true}); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}
	got := readFile(t, path)
	for _, want := range []string{"environment", "TOKEN", "secret", "/new/sting"} {
		if !strings.Contains(got, want) {
			t.Errorf("upgrade dropped %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "/old/sting") {
		t.Errorf("stale command not replaced:\n%s", got)
	}
}

// TestClaudeCreatesPrivateFile ensures a freshly created ~/.claude.json is 0600,
// since Claude Code stores OAuth material there.
func TestClaudeCreatesPrivateFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not model Unix permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	r, _ := ByName("claude")
	if err := r.WriteEntry(path, Entry{Command: "/bin/sting", Args: []string{"mcp"}, Enabled: true}); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}
