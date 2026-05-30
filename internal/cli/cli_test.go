// SPDX-License-Identifier: MIT
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/internal/credentials"
	"github.com/skaphos/sting/internal/mcpinstall"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// isolateHome points HOME at a fresh temp dir and clears the env vars that
// influence runtime auto-detection, so detection finds nothing unless the test
// creates marker files itself.
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

// newCmd builds a bare cobra command wired with in/out/err buffers for direct
// RunE invocation, free of the global command tree's persistent state.
func newCmd() (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	cmd := &cobra.Command{Use: "test"}
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	return cmd, &out, &errBuf
}

func TestPick(t *testing.T) {
	if got := pick("value", "fb"); got != "value" {
		t.Errorf("pick(value) = %q, want value", got)
	}
	if got := pick("", "fb"); got != "fb" {
		t.Errorf("pick(empty) = %q, want fb", got)
	}
	if got := pick("   ", "fb"); got != "fb" {
		t.Errorf("pick(spaces) = %q, want fb", got)
	}
}

func TestDash(t *testing.T) {
	if got := dash(""); got != "-" {
		t.Errorf("dash(empty) = %q, want -", got)
	}
	if got := dash("x"); got != "x" {
		t.Errorf("dash(x) = %q, want x", got)
	}
}

func TestParseInstallScope(t *testing.T) {
	cases := []struct {
		in      string
		want    mcpinstall.Scope
		wantErr bool
	}{
		{"user", mcpinstall.ScopeUser, false},
		{"", mcpinstall.ScopeUser, false},
		{"project", mcpinstall.ScopeProject, false},
		{"PROJECT", mcpinstall.ScopeProject, false},
		{"bogus", 0, true},
	}
	for _, c := range cases {
		got, err := parseInstallScope(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseInstallScope(%q): expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseInstallScope(%q): unexpected error %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("parseInstallScope(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestConfigMissing(t *testing.T) {
	if !configMissing(viper.ConfigFileNotFoundError{}) {
		t.Error("configMissing(ConfigFileNotFoundError) = false, want true")
	}
	notExist := &fs.PathError{Op: "open", Path: "/nope", Err: fs.ErrNotExist}
	if !configMissing(notExist) {
		t.Error("configMissing(PathError ErrNotExist) = false, want true")
	}
	if configMissing(errors.New("some parse error")) {
		t.Error("configMissing(unrelated) = true, want false")
	}
}

func TestConfigSearchDirs(t *testing.T) {
	home := isolateHome(t)
	dirs := configSearchDirs()
	// Without XDG_CONFIG_HOME, expect the two HOME-based dirs plus ".".
	wantHome1 := filepath.Join(home, ".config", "sting")
	wantHome2 := filepath.Join(home, ".sting")
	if !contains(dirs, wantHome1) || !contains(dirs, wantHome2) {
		t.Errorf("configSearchDirs() = %v, missing HOME dirs", dirs)
	}
	if dirs[len(dirs)-1] != "." {
		t.Errorf("configSearchDirs() last = %q, want .", dirs[len(dirs)-1])
	}

	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	dirs = configSearchDirs()
	wantXDG := filepath.Join(xdg, "sting")
	if dirs[0] != wantXDG {
		t.Errorf("configSearchDirs() first = %q, want %q", dirs[0], wantXDG)
	}
}

func contains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}

func TestDesiredInstallEntry(t *testing.T) {
	// Empty override resolves to os.Executable().
	entry, err := desiredInstallEntry("")
	if err != nil {
		t.Fatalf("desiredInstallEntry(empty): %v", err)
	}
	if entry.Command == "" {
		t.Error("desiredInstallEntry(empty): empty command")
	}
	if len(entry.Args) != 1 || entry.Args[0] != "mcp" {
		t.Errorf("desiredInstallEntry args = %v, want [mcp]", entry.Args)
	}
	if !entry.Enabled {
		t.Error("desiredInstallEntry: not enabled")
	}

	// Explicit override is used verbatim (after trimming).
	entry, err = desiredInstallEntry("  /tmp/sting  ")
	if err != nil {
		t.Fatalf("desiredInstallEntry(override): %v", err)
	}
	if entry.Command != "/tmp/sting" {
		t.Errorf("desiredInstallEntry override = %q, want /tmp/sting", entry.Command)
	}
}

func TestRuntimeSelection(t *testing.T) {
	cmd, _, _ := newCmd()
	addRuntimeFlags(cmd)
	if err := cmd.Flags().Set("claude", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("grok", "true"); err != nil {
		t.Fatal(err)
	}
	sel := runtimeSelection(cmd)
	if len(sel.Explicit) != 2 {
		t.Fatalf("Explicit = %v, want 2 entries", sel.Explicit)
	}
	if sel.Explicit[0] != "claude" || sel.Explicit[1] != "grok" {
		t.Errorf("Explicit = %v, want [claude grok]", sel.Explicit)
	}

	// No flags set: empty Explicit (auto-detect).
	cmd2, _, _ := newCmd()
	addRuntimeFlags(cmd2)
	if sel := runtimeSelection(cmd2); len(sel.Explicit) != 0 {
		t.Errorf("Explicit = %v, want empty", sel.Explicit)
	}
}

func TestWriteInstallListTable(t *testing.T) {
	var buf bytes.Buffer
	rows := []listRow{
		{Name: "claude", Scope: "user", Path: "/home/x/.claude.json", State: "registered", Command: "/bin/sting"},
		{Name: "codex", Scope: "user", Path: "", State: "not registered"},
	}
	if err := writeInstallListTable(&buf, rows); err != nil {
		t.Fatalf("writeInstallListTable: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"NAME", "SCOPE", "PATH", "STATE", "COMMAND", "claude", "registered", "not registered"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}
	// Empty path/command rendered as dash.
	if !strings.Contains(out, "-") {
		t.Errorf("table output missing dash for empty fields:\n%s", out)
	}
}

func TestPrintClaudePermissionsBlock(t *testing.T) {
	var buf bytes.Buffer
	if err := printClaudePermissionsBlock(&buf); err != nil {
		t.Fatalf("printClaudePermissionsBlock: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "mcp__sting__get_commits") {
		t.Errorf("permissions block missing tool name:\n%s", out)
	}
}

func TestPrintManualSnippets(t *testing.T) {
	desired := mcpinstall.Entry{Command: "/bin/sting", Args: []string{"mcp"}, Enabled: true}

	// target "all": all runtimes plus the claude permissions block.
	var all bytes.Buffer
	if err := printManualSnippets(&all, "all", desired); err != nil {
		t.Fatalf("printManualSnippets(all): %v", err)
	}
	for _, name := range []string{"claude", "codex", "opencode", "grok"} {
		if !strings.Contains(all.String(), "# "+name) {
			t.Errorf("manual(all) missing # %s:\n%s", name, all.String())
		}
	}
	if !strings.Contains(all.String(), "mcp__sting__get_commits") {
		t.Error("manual(all) missing claude permissions block")
	}

	// Single runtime.
	var one bytes.Buffer
	if err := printManualSnippets(&one, "codex", desired); err != nil {
		t.Fatalf("printManualSnippets(codex): %v", err)
	}
	if !strings.Contains(one.String(), "# codex") {
		t.Errorf("manual(codex) missing snippet:\n%s", one.String())
	}
	if strings.Contains(one.String(), "# claude") {
		t.Error("manual(codex) should not include claude")
	}

	// Empty target defaults to all.
	var empty bytes.Buffer
	if err := printManualSnippets(&empty, "", desired); err != nil {
		t.Fatalf("printManualSnippets(empty): %v", err)
	}
	if !strings.Contains(empty.String(), "# claude") {
		t.Error("manual(empty) should default to all")
	}

	// Invalid target errors.
	var bad bytes.Buffer
	if err := printManualSnippets(&bad, "bogus", desired); err == nil {
		t.Error("printManualSnippets(bogus): expected error")
	}
}

// --- runInstall ---

func registerInstallFlags(cmd *cobra.Command) {
	addRuntimeFlags(cmd)
	cmd.Flags().String("scope", "user", "")
	cmd.Flags().String("command", "", "")
	cmd.Flags().String("manual", "", "")
	cmd.Flags().Lookup("manual").NoOptDefVal = "all"
}

func TestRunInstallNoRuntimeDetected(t *testing.T) {
	isolateHome(t)
	cmd, _, _ := newCmd()
	registerInstallFlags(cmd)
	err := runInstall(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "no MCP-capable runtime detected") {
		t.Fatalf("runInstall: got %v, want no-runtime error", err)
	}
}

func TestRunInstallManual(t *testing.T) {
	isolateHome(t)
	cmd, out, _ := newCmd()
	registerInstallFlags(cmd)
	if err := cmd.Flags().Set("manual", "all"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("command", "/tmp/x"); err != nil {
		t.Fatal(err)
	}
	if err := runInstall(cmd, nil); err != nil {
		t.Fatalf("runInstall(manual): %v", err)
	}
	if !strings.Contains(out.String(), "# claude") {
		t.Errorf("manual install missing snippets:\n%s", out.String())
	}
}

func TestRunInstallClaudeLifecycle(t *testing.T) {
	home := isolateHome(t)
	cfgPath := filepath.Join(home, ".claude.json")

	run := func() string {
		cmd, out, _ := newCmd()
		registerInstallFlags(cmd)
		if err := cmd.Flags().Set("claude", "true"); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Flags().Set("command", "/tmp/x"); err != nil {
			t.Fatal(err)
		}
		if err := runInstall(cmd, nil); err != nil {
			t.Fatalf("runInstall(claude): %v", err)
		}
		return out.String()
	}

	if got := run(); !strings.Contains(got, "registered") {
		t.Errorf("first install: got %q, want registered", got)
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("expected %s written: %v", cfgPath, err)
	}
	if got := run(); !strings.Contains(got, "unchanged") {
		t.Errorf("second install: got %q, want unchanged", got)
	}

	// Change command -> updated.
	cmd, out, _ := newCmd()
	registerInstallFlags(cmd)
	if err := cmd.Flags().Set("claude", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("command", "/tmp/y"); err != nil {
		t.Fatal(err)
	}
	if err := runInstall(cmd, nil); err != nil {
		t.Fatalf("runInstall(updated): %v", err)
	}
	if !strings.Contains(out.String(), "updated") {
		t.Errorf("third install: got %q, want updated", out.String())
	}
}

func TestRunInstallInvalidScope(t *testing.T) {
	isolateHome(t)
	cmd, _, _ := newCmd()
	registerInstallFlags(cmd)
	if err := cmd.Flags().Set("scope", "bogus"); err != nil {
		t.Fatal(err)
	}
	if err := runInstall(cmd, nil); err == nil {
		t.Fatal("runInstall(bogus scope): expected error")
	}
}

// --- runInstallList ---

func TestRunInstallListTable(t *testing.T) {
	isolateHome(t)
	cmd, out, _ := newCmd()
	cmd.Flags().String("scope", "user", "")
	cmd.Flags().Bool("json", false, "")
	if err := runInstallList(cmd, nil); err != nil {
		t.Fatalf("runInstallList(table): %v", err)
	}
	for _, want := range []string{"NAME", "SCOPE", "PATH", "STATE"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("list table missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunInstallListJSON(t *testing.T) {
	isolateHome(t)
	cmd, out, _ := newCmd()
	cmd.Flags().String("scope", "user", "")
	cmd.Flags().Bool("json", false, "")
	if err := cmd.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	if err := runInstallList(cmd, nil); err != nil {
		t.Fatalf("runInstallList(json): %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out.String())
	}
	if _, ok := doc["runtimes"]; !ok {
		t.Errorf("JSON output missing runtimes key: %v", doc)
	}
}

func TestRunInstallListStale(t *testing.T) {
	isolateHome(t)
	installClaude(t) // writes command "/tmp/x", which differs from os.Executable()

	cmd, out, _ := newCmd()
	cmd.Flags().String("scope", "user", "")
	cmd.Flags().Bool("json", false, "")
	if err := runInstallList(cmd, nil); err != nil {
		t.Fatalf("runInstallList(stale): %v", err)
	}
	if !strings.Contains(out.String(), "registered (stale)") {
		t.Errorf("list should report stale registration:\n%s", out.String())
	}
}

func TestRunInstallListProjectScope(t *testing.T) {
	isolateHome(t)
	cmd, out, _ := newCmd()
	cmd.Flags().String("scope", "project", "")
	cmd.Flags().Bool("json", false, "")
	if err := runInstallList(cmd, nil); err != nil {
		t.Fatalf("runInstallList(project): %v", err)
	}
	// Codex has no project scope, so it reports unsupported.
	if !strings.Contains(out.String(), "unsupported") {
		t.Errorf("project list should mark codex unsupported:\n%s", out.String())
	}
}

func TestRunInstallListInvalidScope(t *testing.T) {
	isolateHome(t)
	cmd, _, _ := newCmd()
	cmd.Flags().String("scope", "bogus", "")
	cmd.Flags().Bool("json", false, "")
	if err := runInstallList(cmd, nil); err == nil {
		t.Fatal("runInstallList(bogus scope): expected error")
	}
}

// --- runUninstall ---

func registerUninstallFlags(cmd *cobra.Command) {
	addRuntimeFlags(cmd)
	cmd.Flags().String("scope", "user", "")
	cmd.Flags().Bool("yes", false, "")
}

// installClaude writes a sting entry into the isolated HOME's claude config so
// uninstall has something to remove.
func installClaude(t *testing.T) {
	t.Helper()
	cmd, _, _ := newCmd()
	registerInstallFlags(cmd)
	if err := cmd.Flags().Set("claude", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("command", "/tmp/x"); err != nil {
		t.Fatal(err)
	}
	if err := runInstall(cmd, nil); err != nil {
		t.Fatalf("install setup: %v", err)
	}
}

func TestRunUninstallYes(t *testing.T) {
	isolateHome(t)
	installClaude(t)

	cmd, out, _ := newCmd()
	registerUninstallFlags(cmd)
	if err := cmd.Flags().Set("claude", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	if err := runUninstall(cmd, nil); err != nil {
		t.Fatalf("runUninstall(yes): %v", err)
	}
	if !strings.Contains(out.String(), "removed") {
		t.Errorf("uninstall(yes): got %q, want removed", out.String())
	}
}

func TestRunUninstallConfirmYes(t *testing.T) {
	isolateHome(t)
	installClaude(t)

	cmd, out, _ := newCmd()
	registerUninstallFlags(cmd)
	cmd.SetIn(strings.NewReader("y\n"))
	if err := cmd.Flags().Set("claude", "true"); err != nil {
		t.Fatal(err)
	}
	if err := runUninstall(cmd, nil); err != nil {
		t.Fatalf("runUninstall(confirm y): %v", err)
	}
	if !strings.Contains(out.String(), "removed") {
		t.Errorf("uninstall(confirm y): got %q, want removed", out.String())
	}
}

func TestRunUninstallConfirmNo(t *testing.T) {
	isolateHome(t)
	installClaude(t)

	cmd, out, _ := newCmd()
	registerUninstallFlags(cmd)
	cmd.SetIn(strings.NewReader("n\n"))
	if err := cmd.Flags().Set("claude", "true"); err != nil {
		t.Fatal(err)
	}
	if err := runUninstall(cmd, nil); err != nil {
		t.Fatalf("runUninstall(confirm n): %v", err)
	}
	if !strings.Contains(out.String(), "uninstall cancelled") {
		t.Errorf("uninstall(confirm n): got %q, want cancelled", out.String())
	}
}

func TestRunUninstallNothingPresent(t *testing.T) {
	isolateHome(t)
	cmd, _, _ := newCmd()
	registerUninstallFlags(cmd)
	if err := cmd.Flags().Set("claude", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	// No entry present -> no targets -> nil, no output.
	if err := runUninstall(cmd, nil); err != nil {
		t.Fatalf("runUninstall(nothing): %v", err)
	}
}

func TestRunUninstallNoRuntime(t *testing.T) {
	isolateHome(t)
	cmd, _, _ := newCmd()
	registerUninstallFlags(cmd)
	if err := runUninstall(cmd, nil); err == nil || !strings.Contains(err.Error(), "no MCP-capable runtime detected") {
		t.Fatalf("runUninstall: got %v, want no-runtime error", err)
	}
}

func TestRunUninstallInvalidScope(t *testing.T) {
	isolateHome(t)
	cmd, _, _ := newCmd()
	registerUninstallFlags(cmd)
	if err := cmd.Flags().Set("scope", "bogus"); err != nil {
		t.Fatal(err)
	}
	if err := runUninstall(cmd, nil); err == nil {
		t.Fatal("runUninstall(bogus scope): expected error")
	}
}

func TestRunUninstallUnsupportedScopeExplicit(t *testing.T) {
	isolateHome(t)
	cmd, _, _ := newCmd()
	registerUninstallFlags(cmd)
	if err := cmd.Flags().Set("codex", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("scope", "project"); err != nil {
		t.Fatal(err)
	}
	err := runUninstall(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("runUninstall(codex project): got %v, want unsupported-scope error", err)
	}
}

func TestRunInstallUnsupportedScopeExplicit(t *testing.T) {
	isolateHome(t)
	cmd, _, _ := newCmd()
	registerInstallFlags(cmd)
	if err := cmd.Flags().Set("codex", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("scope", "project"); err != nil {
		t.Fatal(err)
	}
	err := runInstall(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("runInstall(codex project): got %v, want unsupported-scope error", err)
	}
}

func TestConfirm(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"yes\n", true},
		{"y\n", true},
		{"n\n", false},
		{"", false},
		{"garbage\n", false},
	}
	for _, c := range cases {
		cmd, _, _ := newCmd()
		cmd.SetIn(strings.NewReader(c.in))
		got, err := confirm(cmd, "prompt: ")
		if err != nil {
			t.Errorf("confirm(%q): unexpected error %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("confirm(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// --- runQuery ---

// seedValidConfig replaces the package viper with one carrying a minimally
// valid config so loadConfig() succeeds, letting runQuery reach its later
// branches. It restores the original after the test.
func seedValidConfig(t *testing.T) {
	t.Helper()
	orig := v
	t.Cleanup(func() { v = orig })
	nv := viper.New()
	for k, val := range config.Defaults() {
		nv.SetDefault(k, val)
	}
	v = nv
}

func TestAuthStatusOutput_NoCredentials(t *testing.T) {
	cmd, out, _ := newCmd()

	// Force a clean credential store for this test
	t.Setenv("GH_CONFIG_DIR", t.TempDir())

	err := runAuthStatus(cmd, nil)
	if err != nil {
		t.Fatalf("runAuthStatus returned error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Not logged in") {
		t.Errorf("expected 'Not logged in' messaging, got:\n%s", output)
	}
}

func TestAuthStatusOutput_VariousStates(t *testing.T) {
	cases := []struct {
		name       string
		setup      func()
		wantSubstr []string
	}{
		{
			name: "legacy github only",
			setup: func() {
				t.Setenv("GH_CONFIG_DIR", t.TempDir())
				// Simulate legacy token via viper (the global v in root.go)
				v.Set("token", "legacy-gh-pat")
			},
			wantSubstr: []string{"Legacy token available via STING_TOKEN"},
		},
		{
			name: "legacy gitlab only",
			setup: func() {
				t.Setenv("GH_CONFIG_DIR", t.TempDir())
				v.Set("gitlab_token", "legacy-gl-pat")
			},
			wantSubstr: []string{"Legacy token available via STING_GITLAB_TOKEN"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, out, _ := newCmd()
			tc.setup()
			defer func() {
				v.Set("token", "")
				v.Set("gitlab_token", "")
			}()

			_ = runAuthStatus(cmd, nil)
			output := out.String()
			for _, want := range tc.wantSubstr {
				if !strings.Contains(output, want) {
					t.Errorf("output missing %q:\n%s", want, output)
				}
			}
		})
	}
}

func TestRunQueryNoAuthorShowsHelp(t *testing.T) {
	cmd, out, _ := newCmd()
	cmd.Short = "test command"
	registerQueryFlags(cmd)
	if err := runQuery(cmd, nil); err != nil {
		t.Fatalf("runQuery(no author): %v", err)
	}
	if out.Len() == 0 {
		t.Error("runQuery(no author): expected help output")
	}
}

func TestRunQueryInvalidFormat(t *testing.T) {
	seedValidConfig(t)
	cmd, _, _ := newCmd()
	registerQueryFlags(cmd)
	if err := cmd.Flags().Set("author", "octocat"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("format", "bogus"); err != nil {
		t.Fatal(err)
	}
	if err := runQuery(cmd, nil); err == nil {
		t.Fatal("runQuery(bogus format): expected error")
	}
}

func TestRunQueryInvalidScope(t *testing.T) {
	seedValidConfig(t)
	cmd, _, _ := newCmd()
	registerQueryFlags(cmd)
	if err := cmd.Flags().Set("author", "octocat"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("scope", "bogus"); err != nil {
		t.Fatal(err)
	}
	if err := runQuery(cmd, nil); err == nil {
		t.Fatal("runQuery(bogus scope): expected error")
	}
}

func TestRunQueryLoadConfigError(t *testing.T) {
	// Default package viper v has no defaults seeded here, so per_page is 0 and
	// Validate() fails inside loadConfig.
	orig := v
	t.Cleanup(func() { v = orig })
	v = viper.New()
	cmd, _, _ := newCmd()
	registerQueryFlags(cmd)
	if err := cmd.Flags().Set("author", "octocat"); err != nil {
		t.Fatal(err)
	}
	if err := runQuery(cmd, nil); err == nil {
		t.Fatal("runQuery(bad config): expected error")
	}
}

func TestLoadConfigValid(t *testing.T) {
	seedValidConfig(t)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.PerPage != 100 {
		t.Errorf("PerPage = %d, want 100", cfg.PerPage)
	}
}

// --- Execute / initConfig / must ---

func TestExecuteVersion(t *testing.T) {
	isolateHome(t)
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })
	os.Args = []string{"sting", "version"}
	// version's Run returns no error and does not call os.Exit, so Execute
	// returns cleanly.
	Execute()
}

func TestMustPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("must(err): expected panic")
		}
	}()
	must(errors.New("boom"))
}

func TestMustNoPanic(t *testing.T) {
	must(nil) // should not panic
}

func TestRunInit(t *testing.T) {
	cmd, out, _ := newCmd()
	err := runInit(cmd, nil)
	if err != nil {
		t.Fatalf("runInit: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "Welcome to Sting") {
		t.Errorf("expected welcome message from init, got:\n%s", output)
	}
}

func TestRunInit_AlreadyHasGitHub(t *testing.T) {
	// Basic smoke test for the already-authenticated GitHub path.
	cmd, out, _ := newCmd()
	err := runInit(cmd, nil)
	if err != nil {
		t.Fatalf("runInit: %v", err)
	}
	if !strings.Contains(out.String(), "Welcome to Sting") {
		t.Errorf("expected welcome message")
	}
}

func TestInitSubcommandsExist(t *testing.T) {
	// Verify the subcommand structure works
	_, _, _ = newCmd()
	initCmd.AddCommand(initGitHubCmd, initGitLabCmd) // ensure registered

	if initGitHubCmd.Use != "github" || initGitLabCmd.Use != "gitlab" {
		t.Error("expected github and gitlab subcommands under init")
	}
}

// --- GitLab auth command tests ---

func TestRunAuthGitLab_SelfHostedRequiresOwnApp(t *testing.T) {
	// Ensure clean globals for this test
	origID := authGitLabClientID
	origHost := authGitLabHostname
	origWithToken := authGitLabWithToken
	t.Cleanup(func() {
		authGitLabClientID = origID
		authGitLabHostname = origHost
		authGitLabWithToken = origWithToken
	})

	authGitLabClientID = ""
	authGitLabHostname = "gitlab.example.com"
	authGitLabWithToken = false

	cmd, out, _ := newCmd()

	err := runAuthGitLab(cmd, nil)
	if err == nil {
		t.Fatal("expected error when using default creds on self-hosted GitLab")
	}

	msg := err.Error()
	if !strings.Contains(msg, "Self-hosted GitLab detected") {
		t.Errorf("error should mention self-hosted detection, got: %v", err)
	}
	if !strings.Contains(msg, "gitlab.example.com") {
		t.Errorf("error should mention the hostname, got: %v", err)
	}
	if !strings.Contains(msg, "register an OAuth Application") {
		t.Errorf("error should instruct user to register their own app, got: %v", err)
	}
	if !strings.Contains(msg, "Device authorization grant flow") {
		t.Errorf("error should mention enabling device flow, got: %v", err)
	}

	// Output should be empty for error path (message is in the returned error)
	_ = out
}

func TestRunAuthStatus_WithStoredCredentials(t *testing.T) {
	t.Setenv("GH_CONFIG_DIR", t.TempDir())

	// Pre-populate some credentials via the store
	store, _ := credentials.New()
	_, _ = store.Save(context.Background(), credentials.ProviderGitHub, "github.com", credentials.Token{
		Type:        credentials.TokenTypeOAuth,
		AccessToken: "gho_test123",
		Username:    "octocat",
	}, false)

	cmd, out, _ := newCmd()
	err := runAuthStatus(cmd, nil)
	if err != nil {
		t.Fatalf("runAuthStatus: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "github.com") && !strings.Contains(output, "Logged in") {
		t.Errorf("expected to see stored credential in status output, got:\n%s", output)
	}
}

func TestRunAuthGitLab_WithToken(t *testing.T) {
	// Isolate storage. Use --insecure-storage so the test is hermetic even
	// when no keyring (org.freedesktop.secrets) is available in CI.
	t.Setenv("GH_CONFIG_DIR", t.TempDir())

	origWithToken := authGitLabWithToken
	origInsecure := authGitLabInsecure
	t.Cleanup(func() {
		authGitLabWithToken = origWithToken
		authGitLabInsecure = origInsecure
	})

	authGitLabWithToken = true
	authGitLabInsecure = true
	authGitLabHostname = "" // defaults to gitlab.com inside func

	cmd, out, _ := newCmd()
	cmd.SetIn(strings.NewReader("glpat-test-token-123\n"))

	err := runAuthGitLab(cmd, nil)
	if err != nil {
		t.Fatalf("runAuthGitLab --with-token failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "GitLab token stored (insecure fallback)") {
		t.Errorf("expected insecure storage message, got:\n%s", output)
	}
	if !strings.Contains(output, "Host: gitlab.com") {
		t.Errorf("expected host in output, got:\n%s", output)
	}
	if !strings.Contains(output, "sting auth status") {
		t.Errorf("expected status hint in output, got:\n%s", output)
	}
}

func TestRunAuthLogout_Idempotent(t *testing.T) {
	t.Setenv("GH_CONFIG_DIR", t.TempDir())

	origHost := authLogoutHostname
	t.Cleanup(func() {
		authLogoutHostname = origHost
	})

	authLogoutHostname = ""

	cmd, out, _ := newCmd()

	err := runAuthLogout(cmd, nil)
	if err != nil {
		t.Fatalf("runAuthLogout with no credentials should be idempotent: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "No credentials") && !strings.Contains(output, "Logged out") {
		t.Logf("logout no-op output: %s", output)
	}
}

func TestRunAuthLogout_SpecificProvider(t *testing.T) {
	t.Setenv("GH_CONFIG_DIR", t.TempDir())

	origHost := authLogoutHostname
	t.Cleanup(func() {
		authLogoutHostname = origHost
	})

	authLogoutHostname = ""

	cmd, out, _ := newCmd()

	err := runAuthLogout(cmd, []string{"github"})
	if err != nil {
		t.Fatalf("runAuthLogout github: %v", err)
	}
	_ = out
}

func TestRunAuthGitLab_WithToken_EmptyInput(t *testing.T) {
	t.Setenv("GH_CONFIG_DIR", t.TempDir())

	orig := authGitLabWithToken
	t.Cleanup(func() { authGitLabWithToken = orig })
	authGitLabWithToken = true

	cmd, _, _ := newCmd()
	cmd.SetIn(strings.NewReader("\n\n")) // only whitespace

	err := runAuthGitLab(cmd, nil)
	if err == nil {
		t.Fatal("expected error for empty token on stdin")
	}
	if !strings.Contains(err.Error(), "no token provided on stdin") {
		t.Errorf("expected 'no token provided' error, got: %v", err)
	}
}

func TestRunAuthStatus_MultipleHosts(t *testing.T) {
	t.Setenv("GH_CONFIG_DIR", t.TempDir())

	store, _ := credentials.New()
	_, _ = store.Save(context.Background(), credentials.ProviderGitHub, "github.com", credentials.Token{AccessToken: "main"}, false)
	_, _ = store.Save(context.Background(), credentials.ProviderGitHub, "ghe.example.com", credentials.Token{AccessToken: "enterprise"}, false)

	cmd, out, _ := newCmd()
	_ = runAuthStatus(cmd, nil)
	output := out.String()
	if !strings.Contains(output, "github.com") || !strings.Contains(output, "ghe.example.com") {
		t.Errorf("expected multiple hosts in status output, got:\n%s", output)
	}
}

func TestFetchGitLabUsername(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{
			name: "happy path",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer testtok" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"username": "alice"})
			},
			want: "alice",
		},
		{
			name: "non-200",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			want: "",
		},
		{
			name: "bad json",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`not json`))
			},
			want: "",
		},
		{
			name: "missing username field",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"id": "123"})
			},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(tc.handler)
			defer ts.Close()

			got := fetchGitLabUsername(ts.URL, "testtok")
			if got != tc.want {
				t.Errorf("fetchGitLabUsername() = %q, want %q", got, tc.want)
			}
		})
	}
}
