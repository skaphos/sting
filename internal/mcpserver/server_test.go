// SPDX-License-Identifier: MIT
package mcpserver

import (
	"slices"
	"testing"

	"github.com/skaphos/sting/internal/config"
)

// TestReadOnlyTools pins the read-only tool set. sting is read-only by design,
// so every tool it exposes must appear here; if a mutating tool is ever added,
// this test should fail until the installer's auto-approve list is reconsidered.
func TestReadOnlyTools(t *testing.T) {
	got := ReadOnlyTools()
	want := []string{"get_commits"}
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
