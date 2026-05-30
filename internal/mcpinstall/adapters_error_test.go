// SPDX-License-Identifier: MIT
package mcpinstall

import (
	"os"
	"path/filepath"
	"testing"
)

// readEntryCase drives an adapter's ReadEntry against a fixture file.
type readEntryCase struct {
	fixture     string
	wantErr     bool
	wantPresent bool
	wantCommand string
}

func runReadEntryCases(t *testing.T, name string, cases []readEntryCase) {
	t.Helper()
	r, ok := ByName(name)
	if !ok {
		t.Fatalf("adapter %q not registered", name)
	}
	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			entry, present, err := r.ReadEntry(c.fixture)
			if c.wantErr {
				if err == nil {
					t.Fatalf("ReadEntry(%s) expected error, got present=%v entry=%+v", c.fixture, present, entry)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadEntry(%s): %v", c.fixture, err)
			}
			if present != c.wantPresent {
				t.Errorf("ReadEntry(%s) present = %v, want %v", c.fixture, present, c.wantPresent)
			}
			if c.wantPresent && c.wantCommand != "" && entry.Command != c.wantCommand {
				t.Errorf("ReadEntry(%s) command = %q, want %q", c.fixture, entry.Command, c.wantCommand)
			}
		})
	}
}

func TestClaudeReadEntry(t *testing.T) {
	runReadEntryCases(t, "claude", []readEntryCase{
		{fixture: "testdata/claude/malformed.json", wantErr: true},
		{fixture: "testdata/claude/mcpservers-not-object.json", wantErr: true},
		{fixture: "testdata/claude/entry-not-object.json", wantErr: true},
		{fixture: "testdata/claude/existing-match.json", wantPresent: true, wantCommand: "/usr/local/bin/sting"},
		{fixture: "testdata/claude/existing-stale.json", wantPresent: true, wantCommand: "/old/path/sting"},
		{fixture: "testdata/claude/other-servers.json", wantPresent: false},
		{fixture: "testdata/claude/empty.json", wantPresent: false},
		{fixture: "testdata/claude/does-not-exist.json", wantPresent: false},
	})
}

func TestCodexReadEntry(t *testing.T) {
	runReadEntryCases(t, "codex", []readEntryCase{
		{fixture: "testdata/codex/malformed.toml", wantErr: true},
		{fixture: "testdata/codex/mcpservers-not-table.toml", wantErr: true},
		{fixture: "testdata/codex/entry-not-table.toml", wantErr: true},
		{fixture: "testdata/codex/existing-match.toml", wantPresent: true, wantCommand: "/usr/local/bin/sting"},
		{fixture: "testdata/codex/existing-stale.toml", wantPresent: true, wantCommand: "/old/path/sting"},
		{fixture: "testdata/codex/other-servers.toml", wantPresent: false},
		{fixture: "testdata/codex/empty.toml", wantPresent: false},
		{fixture: "testdata/codex/does-not-exist.toml", wantPresent: false},
	})
}

func TestGrokReadEntry(t *testing.T) {
	runReadEntryCases(t, "grok", []readEntryCase{
		{fixture: "testdata/grok/malformed.toml", wantErr: true},
		{fixture: "testdata/grok/mcpservers-not-table.toml", wantErr: true},
		{fixture: "testdata/grok/entry-not-table.toml", wantErr: true},
		{fixture: "testdata/grok/existing-match.toml", wantPresent: true, wantCommand: "/usr/local/bin/sting"},
		{fixture: "testdata/grok/existing-stale.toml", wantPresent: true, wantCommand: "/old/path/sting"},
		{fixture: "testdata/grok/other-servers.toml", wantPresent: false},
		{fixture: "testdata/grok/empty.toml", wantPresent: false},
		{fixture: "testdata/grok/does-not-exist.toml", wantPresent: false},
	})
}

func TestGrokReadEntryEnabledFlag(t *testing.T) {
	r, _ := ByName("grok")
	entry, present, err := r.ReadEntry("testdata/grok/existing-stale.toml")
	if err != nil || !present {
		t.Fatalf("ReadEntry stale: present=%v err=%v", present, err)
	}
	if entry.Enabled {
		t.Error("stale grok entry should have Enabled=false")
	}
}

func TestOpencodeReadEntry(t *testing.T) {
	runReadEntryCases(t, "opencode", []readEntryCase{
		{fixture: "testdata/opencode/malformed.json", wantErr: true},
		{fixture: "testdata/opencode/mcp-not-object.json", wantErr: true},
		{fixture: "testdata/opencode/entry-not-object.json", wantErr: true},
		{fixture: "testdata/opencode/empty-command.json", wantErr: true},
		{fixture: "testdata/opencode/existing-match.json", wantPresent: true, wantCommand: "/usr/local/bin/sting"},
		{fixture: "testdata/opencode/existing-stale.json", wantPresent: true, wantCommand: "/old/path/sting"},
		{fixture: "testdata/opencode/other-servers.json", wantPresent: false},
		{fixture: "testdata/opencode/empty.json", wantPresent: false},
		{fixture: "testdata/opencode/does-not-exist.json", wantPresent: false},
	})
}

// TestOpencodeJsoncRefusal covers checkJsonc's two refusal branches.
func TestOpencodeJsoncRefusal(t *testing.T) {
	r, _ := ByName("opencode")
	dir := t.TempDir()

	// .jsonc path is refused outright.
	jsoncPath := filepath.Join(dir, "opencode.jsonc")
	if err := os.WriteFile(jsoncPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.ReadEntry(jsoncPath); err == nil {
		t.Error("ReadEntry(.jsonc) expected refusal")
	}
	if err := r.WriteEntry(jsoncPath, Entry{Command: "/bin/sting"}); err == nil {
		t.Error("WriteEntry(.jsonc) expected refusal")
	}
	if _, err := r.RemoveEntry(jsoncPath); err == nil {
		t.Error("RemoveEntry(.jsonc) expected refusal")
	}

	// .json path with a .jsonc sibling is refused.
	jsonPath := filepath.Join(dir, "opencode.json")
	if _, _, err := r.ReadEntry(jsonPath); err == nil {
		t.Error("ReadEntry(.json with .jsonc sibling) expected refusal")
	}
}

// TestOpencodeCheckJsoncNonJSON covers the path that is neither .json nor
// .jsonc (returns nil, proceeds).
func TestOpencodeCheckJsoncNonJSON(t *testing.T) {
	if err := checkJsonc("/tmp/some/config.toml"); err != nil {
		t.Errorf("checkJsonc(non-json) = %v, want nil", err)
	}
}

// TestWriteEntryMkdirAll exercises WriteEntry into a fresh nested path that
// requires the adapter to create parent directories.
func TestWriteEntryMkdirAll(t *testing.T) {
	cases := []struct {
		name string
		rel  string
	}{
		{"claude", filepath.Join("a", "b", ".claude.json")},
		{"codex", filepath.Join("c", "d", "config.toml")},
		{"grok", filepath.Join("e", "f", "config.toml")},
		{"opencode", filepath.Join("g", "h", "opencode.json")},
	}
	want := Entry{Command: "/bin/sting", Args: []string{"mcp"}, Enabled: true}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, _ := ByName(c.name)
			path := filepath.Join(t.TempDir(), c.rel)
			if err := r.WriteEntry(path, want); err != nil {
				t.Fatalf("WriteEntry: %v", err)
			}
			_, present, err := r.ReadEntry(path)
			if err != nil {
				t.Fatalf("ReadEntry: %v", err)
			}
			if !present {
				t.Error("entry not present after write")
			}
		})
	}
}

// TestRemoveEntryKeyAbsent seeds a config with only an unrelated server, then
// removes sting (no-op, removed=false), then writes sting and removes it
// (removed=true).
func TestRemoveEntryKeyAbsent(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
		base    string
	}{
		{"claude", "testdata/claude/other-servers.json", ".claude.json"},
		{"codex", "testdata/codex/other-servers.toml", "config.toml"},
		{"grok", "testdata/grok/other-servers.toml", "config.toml"},
		{"opencode", "testdata/opencode/other-servers.json", "opencode.json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, _ := ByName(c.name)
			data, err := os.ReadFile(c.fixture)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), c.base)
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
			// sting key absent -> no-op.
			removed, err := r.RemoveEntry(path)
			if err != nil {
				t.Fatalf("RemoveEntry (absent): %v", err)
			}
			if removed {
				t.Error("RemoveEntry reported removal when sting absent")
			}
			// Now add sting then remove it.
			if err := r.WriteEntry(path, Entry{Command: "/bin/sting", Enabled: true}); err != nil {
				t.Fatalf("WriteEntry: %v", err)
			}
			removed, err = r.RemoveEntry(path)
			if err != nil {
				t.Fatalf("RemoveEntry (present): %v", err)
			}
			if !removed {
				t.Error("RemoveEntry reported nothing removed when sting present")
			}
		})
	}
}

// TestReadEntryPathIsDir exercises the os.ReadFile non-ENOENT error branch in
// readJSONDoc / readTOMLDoc (reading a directory yields an EISDIR error).
func TestReadEntryPathIsDir(t *testing.T) {
	for _, name := range []string{"claude", "codex", "grok", "opencode"} {
		t.Run(name, func(t *testing.T) {
			r, _ := ByName(name)
			dir := t.TempDir()
			if _, _, err := r.ReadEntry(dir); err == nil {
				t.Errorf("%s ReadEntry(dir) expected error", name)
			}
		})
	}
}

// TestReadWriteRemoveOnMalformed ensures Write/Remove surface parse errors too.
func TestWriteRemoveOnMalformed(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
		base    string
	}{
		{"claude", "testdata/claude/malformed.json", "x.json"},
		{"codex", "testdata/codex/malformed.toml", "x.toml"},
		{"grok", "testdata/grok/malformed.toml", "x.toml"},
		{"opencode", "testdata/opencode/malformed.json", "x.json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, _ := ByName(c.name)
			data, err := os.ReadFile(c.fixture)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), c.base)
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := r.WriteEntry(path, Entry{Command: "/bin/sting"}); err == nil {
				t.Error("WriteEntry on malformed expected error")
			}
			if _, err := r.RemoveEntry(path); err == nil {
				t.Error("RemoveEntry on malformed expected error")
			}
		})
	}
}
