// SPDX-License-Identifier: MIT
package mcpinstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopeString(t *testing.T) {
	cases := []struct {
		scope Scope
		want  string
	}{
		{ScopeUser, "user"},
		{ScopeProject, "project"},
		{Scope(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.scope.String(); got != c.want {
			t.Errorf("Scope(%d).String() = %q, want %q", int(c.scope), got, c.want)
		}
	}
}

func TestAllSortedAndComplete(t *testing.T) {
	all := All()
	if len(all) != 4 {
		t.Fatalf("All() len = %d, want 4", len(all))
	}
	names := make([]string, len(all))
	for i, r := range all {
		names[i] = r.Name()
	}
	want := []string{"claude", "codex", "grok", "opencode"}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("All()[%d] = %q, want %q (full: %v)", i, names[i], want[i], names)
		}
	}
}

func TestByNameHitAndMiss(t *testing.T) {
	r, ok := ByName("claude")
	if !ok {
		t.Fatal("ByName(claude) not found")
	}
	if r.Name() != "claude" {
		t.Errorf("ByName(claude).Name() = %q", r.Name())
	}
	if _, ok := ByName("nope"); ok {
		t.Error("ByName(nope) reported found")
	}
}

func TestSelectionFromFlags(t *testing.T) {
	// All flags on, in deterministic order.
	s := SelectionFromFlags(true, true, true, true)
	want := []string{"claude", "codex", "opencode", "grok"}
	if strings.Join(s.Explicit, ",") != strings.Join(want, ",") {
		t.Errorf("all flags Explicit = %v, want %v", s.Explicit, want)
	}

	// Subset preserves order.
	s = SelectionFromFlags(true, false, false, true)
	if strings.Join(s.Explicit, ",") != "claude,grok" {
		t.Errorf("subset Explicit = %v, want [claude grok]", s.Explicit)
	}

	// No flags -> empty (auto-detect).
	s = SelectionFromFlags(false, false, false, false)
	if len(s.Explicit) != 0 {
		t.Errorf("no flags Explicit = %v, want empty", s.Explicit)
	}
}

func TestSelectionResolveExplicit(t *testing.T) {
	s := Selection{Explicit: []string{"codex", "grok"}}
	out, err := s.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(out) != 2 || out[0].Name() != "codex" || out[1].Name() != "grok" {
		t.Errorf("Resolve order = %v", names(out))
	}
}

func TestSelectionResolveExplicitUnknown(t *testing.T) {
	s := Selection{Explicit: []string{"claude", "bogus"}}
	if _, err := s.Resolve(); err == nil {
		t.Fatal("expected error for unknown runtime")
	}
}

func TestSelectionResolveEmptyDetect(t *testing.T) {
	home := isolateHome(t)

	// No markers present -> nothing detected.
	out, err := Selection{}.Resolve()
	if err != nil {
		t.Fatalf("Resolve (empty home): %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected no runtimes detected, got %v", names(out))
	}

	// Create the claude marker -> claude detected.
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = Selection{}.Resolve()
	if err != nil {
		t.Fatalf("Resolve (claude marker): %v", err)
	}
	found := false
	for _, r := range out {
		if r.Name() == "claude" {
			found = true
		}
	}
	if !found {
		t.Errorf("claude not detected after creating marker, got %v", names(out))
	}
}

func names(rs []Runtime) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Name()
	}
	return out
}
