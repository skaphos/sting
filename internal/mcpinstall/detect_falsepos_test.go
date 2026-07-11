// SPDX-License-Identifier: MIT
package mcpinstall

import (
	"path/filepath"
	"testing"
)

// TestGrokDetectStaleEnvVar is the P2 regression: a GROK_CONFIG_DIR pointing at
// a directory that does not exist must not fabricate a detection.
func TestGrokDetectStaleEnvVar(t *testing.T) {
	isolateHome(t)
	t.Setenv("GROK_CONFIG_DIR", filepath.Join(t.TempDir(), "does-not-exist"))
	mustDetect(t, "grok", false)
}

// TestOpencodeDetectStaleEnvVar is the P2 regression for OpenCode.
func TestOpencodeDetectStaleEnvVar(t *testing.T) {
	isolateHome(t)
	t.Setenv("OPENCODE_CONFIG_DIR", filepath.Join(t.TempDir(), "does-not-exist"))
	mustDetect(t, "opencode", false)
}
