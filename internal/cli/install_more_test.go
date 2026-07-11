// SPDX-License-Identifier: MIT
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunInstallPreservesDisabled is the P2 regression: reinstalling must not
// flip an entry the user deliberately disabled back to enabled.
func TestRunInstallPreservesDisabled(t *testing.T) {
	home := isolateHome(t)
	// Seed a grok config with sting explicitly disabled.
	dir := filepath.Join(home, ".grok")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "config.toml")
	seed := "[mcp_servers.sting]\ncommand = \"/old\"\nargs = [\"mcp\"]\nenabled = false\n"
	if err := os.WriteFile(cfg, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd, _, _ := newCmd()
	registerInstallFlags(cmd)
	if err := cmd.Flags().Set("grok", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("command", "/new"); err != nil {
		t.Fatal(err)
	}
	if err := runInstall(cmd, nil); err != nil {
		t.Fatalf("runInstall: %v", err)
	}

	got, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := string(got)
	if !strings.Contains(out, "enabled = false") {
		t.Errorf("reinstall flipped enabled back on:\n%s", out)
	}
	if !strings.Contains(out, "/new") {
		t.Errorf("command not updated:\n%s", out)
	}
}

// TestRunInstallAggregatesErrors is the P2 regression: a failure on one runtime
// is reported (named) but must not abort installation into the others.
func TestRunInstallAggregatesErrors(t *testing.T) {
	home := isolateHome(t)

	// Malformed claude config makes claude ReadEntry fail.
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A detectable codex tree that should install cleanly.
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd, out, _ := newCmd()
	registerInstallFlags(cmd)
	if err := cmd.Flags().Set("claude", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("codex", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("command", "/new"); err != nil {
		t.Fatal(err)
	}

	err := runInstall(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "claude") {
		t.Errorf("expected aggregated error naming claude, got %v", err)
	}
	if !strings.Contains(out.String(), "codex") {
		t.Errorf("codex install did not proceed despite claude failure:\n%s", out.String())
	}
	// Codex config must actually have the entry written.
	got, rerr := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if rerr != nil {
		t.Fatal(rerr)
	}
	if !strings.Contains(string(got), "mcp_servers.sting") {
		t.Errorf("codex entry not written:\n%s", got)
	}
}

// TestRunUninstallMalformedEntryDoesNotAbortOthers is the P2 regression: one
// runtime whose sting entry is undecodable must not prevent removal from the
// other healthy runtimes.
func TestRunUninstallMalformedEntryDoesNotAbortOthers(t *testing.T) {
	home := isolateHome(t)

	// Healthy claude entry that should be removed.
	installClaude(t)

	// Grok config where the sting entry is present but malformed (wrong shape:
	// enabled is a string, not a bool -> ReadEntry errors).
	gdir := filepath.Join(home, ".grok")
	if err := os.MkdirAll(gdir, 0o755); err != nil {
		t.Fatal(err)
	}
	gcfg := filepath.Join(gdir, "config.toml")
	if err := os.WriteFile(gcfg, []byte("[mcp_servers.sting]\ncommand = \"/x\"\nenabled = \"yes\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd, out, _ := newCmd()
	registerUninstallFlags(cmd)
	if err := cmd.Flags().Set("claude", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("grok", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	// The overall call may return an aggregated error for the malformed grok
	// config, but claude removal must still have happened.
	_ = runUninstall(cmd, nil)

	if !strings.Contains(out.String(), "removed claude") {
		t.Errorf("claude was not removed despite malformed grok entry:\n%s", out.String())
	}
	// Grok's undecodable entry is still removable by key (RemoveEntry only needs
	// key existence), so it should have been removed too.
	got, err := os.ReadFile(gcfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "mcp_servers.sting") {
		t.Errorf("malformed grok sting entry not removed:\n%s", got)
	}
}
