// SPDX-License-Identifier: MIT
package mcpserver

import (
	"slices"
	"testing"

	"github.com/skaphos/sting/config"
)

// TestReadOnlyTools pins the read-only tool set. sting is read-only by design,
// so every tool it exposes must appear here; if a mutating tool is ever added,
// this test should fail until the installer's auto-approve list is reconsidered.
func TestReadOnlyTools(t *testing.T) {
	got := ReadOnlyTools()
	want := []string{"get_commits", "get_repo_activity"}
	if !slices.Equal(got, want) {
		t.Errorf("ReadOnlyTools() = %v, want %v", got, want)
	}
}

// TestServerBuilds ensures the MCP server wires up without error from a default
// config (no token).
func TestServerBuilds(t *testing.T) {
	if _, err := New(config.Default()); err != nil {
		t.Fatalf("New: %v", err)
	}
}

// TestReadOnlyToolsDerivedFromRegistry is what makes Constitution Principle I
// mechanical rather than conventional (ADR 0010, research R6).
//
// ReadOnlyTools feeds the installer's auto-approve list. If it were maintained
// by hand alongside the registrations, a tool could be registered but omitted
// from the list — and the failure would be silent, looking like a runtime quirk
// rather than a bug. Asserting that every defined tool is both annotated
// read-only and present in the derived list turns that class of drift into a
// build failure.
func TestReadOnlyToolsDerivedFromRegistry(t *testing.T) {
	defs := toolDefinitions()
	if len(defs) == 0 {
		t.Fatal("toolDefinitions() is empty; the server exposes no tools")
	}

	readOnly := ReadOnlyTools()
	seen := map[string]bool{}

	for _, def := range defs {
		name := def.tool.Name
		if name == "" {
			t.Error("a tool definition has an empty name")
			continue
		}
		if seen[name] {
			t.Errorf("duplicate tool name %q in the registry", name)
		}
		seen[name] = true

		if def.register == nil {
			t.Errorf("tool %q has no registration function", name)
		}
		if def.tool.Annotations == nil {
			t.Errorf("tool %q has no annotations; every sting tool must declare ReadOnlyHint", name)
			continue
		}
		if !def.tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %q is not annotated ReadOnlyHint: sting is read-only by design, "+
				"so adding a mutating surface is a constitutional amendment, not a feature", name)
		}
		if !slices.Contains(readOnly, name) {
			t.Errorf("tool %q is registered but missing from ReadOnlyTools(); "+
				"the installer would stop auto-approving it", name)
		}
		if def.tool.Description == "" {
			t.Errorf("tool %q has no description; the agent has nothing to select on", name)
		}
	}

	// The reverse direction: nothing may appear in the auto-approve list that
	// the server does not actually register.
	for _, name := range readOnly {
		if !seen[name] {
			t.Errorf("ReadOnlyTools() lists %q, which no tool definition registers", name)
		}
	}
}

// TestServerRegistersEveryDefinedTool confirms the registry is actually walked
// at construction: a definition that New skipped would leave the tool absent
// from tools/list while still appearing in the auto-approve snippet.
func TestServerRegistersEveryDefinedTool(t *testing.T) {
	server, err := New(config.Default())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if server == nil {
		t.Fatal("New returned a nil server")
	}
	// The go-sdk exposes no stable enumeration API, so registration is
	// exercised by construction succeeding for every definition; the drift
	// test above covers the name/annotation invariants.
	for _, def := range toolDefinitions() {
		if def.register == nil {
			t.Errorf("tool %q would not have been registered", def.tool.Name)
		}
	}
}
