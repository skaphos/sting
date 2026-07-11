// SPDX-License-Identifier: MIT
package mcpinstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTemp writes content to a fresh temp file and returns its path.
func writeTemp(t *testing.T, base, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), base)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestCodexWritePreservesComments is the P0 regression: a hand-authored
// config.toml with comments, ordering, and unrelated tables must survive an
// update to the sting entry. Before the surgical rewrite this test fails because
// the read-then-remarshal cycle deleted every comment and reordered keys.
func TestCodexWritePreservesComments(t *testing.T) {
	seed := `# top-of-file comment
model = "gpt-5"  # inline comment on a scalar

[mcp_servers.other]
command = "/bin/other"  # keep this comment

[mcp_servers.sting]
command = "/old/path/sting"
args = ["mcp"]

# this note belongs to the tools table
[tools]
web = true
`
	path := writeTemp(t, "config.toml", seed)
	r, _ := ByName("codex")
	if err := r.WriteEntry(path, Entry{Command: "/new/path/sting", Args: []string{"mcp"}}); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}
	got := readFile(t, path)
	for _, want := range []string{
		"# top-of-file comment",
		`model = "gpt-5"  # inline comment on a scalar`,
		"[mcp_servers.other]",
		"# keep this comment",
		"# this note belongs to the tools table",
		"[tools]",
		"web = true",
		"/new/path/sting",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q after update:\n%s", want, got)
		}
	}
	if strings.Contains(got, "/old/path/sting") {
		t.Errorf("stale command not replaced:\n%s", got)
	}
	// The updated entry must still be readable.
	entry, present, err := r.ReadEntry(path)
	if err != nil || !present {
		t.Fatalf("ReadEntry after update: present=%v err=%v", present, err)
	}
	if entry.Command != "/new/path/sting" {
		t.Errorf("command = %q, want /new/path/sting", entry.Command)
	}
}

// TestGrokWritePreservesEnvOnPathChange is the P1 regression: a user entry
// carrying an env table (holding a token) must survive a command-path change.
func TestGrokWritePreservesEnvOnPathChange(t *testing.T) {
	seed := `# grok config
[mcp_servers.sting]
command = "/old/path/sting"
args = ["mcp"]
enabled = true

[mcp_servers.sting.env]
GITHUB_TOKEN = "super-secret-token"
`
	path := writeTemp(t, "config.toml", seed)
	r, _ := ByName("grok")
	if err := r.WriteEntry(path, Entry{Command: "/new/path/sting", Args: []string{"mcp"}, Enabled: true}); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}
	got := readFile(t, path)
	if !strings.Contains(got, "super-secret-token") {
		t.Errorf("env token lost on path change:\n%s", got)
	}
	if !strings.Contains(got, "GITHUB_TOKEN") {
		t.Errorf("env key lost on path change:\n%s", got)
	}
	if !strings.Contains(got, "/new/path/sting") {
		t.Errorf("command not updated:\n%s", got)
	}
	if strings.Contains(got, "/old/path/sting") {
		t.Errorf("stale command not replaced:\n%s", got)
	}
}

// TestGrokWritePreservesDisabled ensures a path change keeps enabled=false when
// the caller writes an explicitly-disabled entry.
func TestGrokWritePreservesDisabled(t *testing.T) {
	path := writeTemp(t, "config.toml", "[mcp_servers.sting]\ncommand = \"/old\"\nenabled = false\n")
	r, _ := ByName("grok")
	if err := r.WriteEntry(path, Entry{Command: "/new", Args: []string{"mcp"}, Enabled: false}); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}
	entry, present, err := r.ReadEntry(path)
	if err != nil || !present {
		t.Fatalf("ReadEntry: present=%v err=%v", present, err)
	}
	if entry.Enabled {
		t.Error("enabled should remain false")
	}
}

// TestCodexRemovePreservesNeighborComment ensures removing sting keeps a comment
// that leads into the following table.
func TestCodexRemovePreservesNeighborComment(t *testing.T) {
	seed := `[mcp_servers.sting]
command = "/x"
args = ["mcp"]

# comment for the next table
[other]
k = 1
`
	path := writeTemp(t, "config.toml", seed)
	r, _ := ByName("codex")
	removed, err := r.RemoveEntry(path)
	if err != nil || !removed {
		t.Fatalf("RemoveEntry: removed=%v err=%v", removed, err)
	}
	got := readFile(t, path)
	if strings.Contains(got, "mcp_servers.sting") {
		t.Errorf("sting table not removed:\n%s", got)
	}
	for _, want := range []string{"# comment for the next table", "[other]", "k = 1"} {
		if !strings.Contains(got, want) {
			t.Errorf("removal dropped %q:\n%s", want, got)
		}
	}
}

// TestCodexAppendPreservesExisting ensures a fresh sting entry is appended
// without disturbing existing tables.
func TestCodexAppendPreservesExisting(t *testing.T) {
	seed := "# header\n[mcp_servers.other]\ncommand = \"/bin/other\"\n"
	path := writeTemp(t, "config.toml", seed)
	r, _ := ByName("codex")
	if err := r.WriteEntry(path, Entry{Command: "/bin/sting", Args: []string{"mcp"}}); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}
	got := readFile(t, path)
	for _, want := range []string{"# header", "[mcp_servers.other]", "/bin/other", "[mcp_servers.sting]", "/bin/sting"} {
		if !strings.Contains(got, want) {
			t.Errorf("append dropped/omitted %q:\n%s", want, got)
		}
	}
}

// TestScannerIgnoresBracketsInValues ensures brackets in multiline arrays and
// header-looking lines inside multiline strings are never mistaken for table
// headers.
func TestScannerIgnoresBracketsInValues(t *testing.T) {
	seed := `matrix = [
  [1, 2],
  [3, 4],
]
note = """
[not.a.header]
still inside the string
"""

[mcp_servers.sting]
command = "/old"
args = ["mcp"]
`
	path := writeTemp(t, "config.toml", seed)
	r, _ := ByName("codex")
	if err := r.WriteEntry(path, Entry{Command: "/new", Args: []string{"mcp"}}); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}
	got := readFile(t, path)
	for _, want := range []string{"matrix = [", "[1, 2]", "[not.a.header]", "still inside the string", "/new"} {
		if !strings.Contains(got, want) {
			t.Errorf("value content corrupted, missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "/old") {
		t.Errorf("command not updated:\n%s", got)
	}
	// The file must still parse and read back.
	if _, present, err := r.ReadEntry(path); err != nil || !present {
		t.Fatalf("ReadEntry after edit: present=%v err=%v", present, err)
	}
}

// TestScannerHandlesArrayTables ensures [[array-table]] headers parse without
// error and are not matched as sting.
func TestScannerHandlesArrayTables(t *testing.T) {
	seed := `[[products]]
name = "a"

[[products]]
name = "b"

[mcp_servers.sting]
command = "/x"
`
	headers, err := scanTOMLHeaders([]byte(seed), "test")
	if err != nil {
		t.Fatalf("scanTOMLHeaders: %v", err)
	}
	if len(headers) != 3 {
		t.Fatalf("got %d headers, want 3", len(headers))
	}
	spans := matchingSpans([]byte(seed), headers, stingPrefix())
	if len(spans) != 1 {
		t.Errorf("matchingSpans = %d, want 1 (only sting)", len(spans))
	}
}

// TestInlineStingEntryRefused ensures a sting entry authored as a dotted/inline
// key (which the surgical editor cannot safely rewrite) is refused rather than
// silently duplicated or corrupted.
func TestInlineStingEntryRefused(t *testing.T) {
	seed := "mcp_servers.sting = { command = \"/x\" }\n"
	path := writeTemp(t, "config.toml", seed)
	r, _ := ByName("codex")
	if err := r.WriteEntry(path, Entry{Command: "/y", Args: []string{"mcp"}}); err == nil || !strings.Contains(err.Error(), "edit manually") {
		t.Errorf("WriteEntry on inline entry: got %v, want manual-edit refusal", err)
	}
	if _, err := r.RemoveEntry(path); err == nil || !strings.Contains(err.Error(), "edit manually") {
		t.Errorf("RemoveEntry on inline entry: got %v, want manual-edit refusal", err)
	}
}

// TestDeleteAbsentTOMLNoOp ensures removing from a file without a sting entry is
// a clean no-op.
func TestDeleteAbsentTOMLNoOp(t *testing.T) {
	path := writeTemp(t, "config.toml", "[other]\nk = 1\n")
	r, _ := ByName("codex")
	removed, err := r.RemoveEntry(path)
	if err != nil {
		t.Fatalf("RemoveEntry: %v", err)
	}
	if removed {
		t.Error("removed = true for absent sting entry")
	}
}

// TestSplitTOMLKeyQuoted covers quoted key segments and the empty-segment error.
func TestSplitTOMLKeyQuoted(t *testing.T) {
	segs, err := splitTOMLKey(`"mcp_servers".'sting'`)
	if err != nil {
		t.Fatalf("splitTOMLKey: %v", err)
	}
	if len(segs) != 2 || segs[0] != "mcp_servers" || segs[1] != "sting" {
		t.Errorf("segs = %v, want [mcp_servers sting]", segs)
	}
	if _, err := splitTOMLKey("a..b"); err == nil {
		t.Error("splitTOMLKey(a..b) expected empty-segment error")
	}
}

// TestQuotedStingHeaderMatches ensures a quoted table header still matches.
func TestQuotedStingHeaderMatches(t *testing.T) {
	seed := "[\"mcp_servers\".\"sting\"]\ncommand = \"/old\"\n"
	path := writeTemp(t, "config.toml", seed)
	r, _ := ByName("codex")
	if err := r.WriteEntry(path, Entry{Command: "/new", Args: []string{"mcp"}}); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}
	got := readFile(t, path)
	if !strings.Contains(got, "/new") || strings.Contains(got, "/old") {
		t.Errorf("quoted-header entry not updated:\n%s", got)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
