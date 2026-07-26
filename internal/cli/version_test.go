// SPDX-License-Identifier: MIT

package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/skaphos/sting/internal/buildinfo"
)

func TestWriteVersionKnown(t *testing.T) {
	var buf bytes.Buffer
	writeVersion(&buf, buildinfo.Info{
		Version:  "v0.7.0",
		Revision: "9ab1f88",
		Time:     "2026-07-25T10:04:11Z",
		Source:   buildinfo.SourceLDFlags,
	})

	out := buf.String()
	for _, want := range []string{"sting v0.7.0", "commit:  9ab1f88", "built:   2026-07-25T10:04:11Z"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
	if strings.Contains(out, "(modified)") {
		t.Errorf("clean build marked modified\ngot:\n%s", out)
	}
}

// TestWriteVersionModified covers FR-002: a build from a dirty working tree
// must not present itself as a clean build of the underlying revision.
func TestWriteVersionModified(t *testing.T) {
	var buf bytes.Buffer
	writeVersion(&buf, buildinfo.Info{
		Revision: "9ab1f88",
		Modified: true,
		Source:   buildinfo.SourceBuildInfo,
	})

	out := buf.String()
	if !strings.Contains(out, "9ab1f88 (modified)") {
		t.Errorf("dirty build not marked modified\ngot:\n%s", out)
	}
}

// TestWriteVersionUnavailable covers FR-004: when nothing was recorded, say so
// rather than inventing a value.
func TestWriteVersionUnavailable(t *testing.T) {
	var buf bytes.Buffer
	writeVersion(&buf, buildinfo.Info{Source: buildinfo.SourceUnknown})

	out := buf.String()
	if !strings.Contains(out, "sting (version unavailable)") {
		t.Errorf("missing unavailable marker\ngot:\n%s", out)
	}
	if !strings.Contains(out, "commit:  unknown") || !strings.Contains(out, "built:   unknown") {
		t.Errorf("unrecorded fields not reported as unknown\ngot:\n%s", out)
	}
	for _, forbidden := range []string{"sting dev", "(devel)"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("output contained placeholder %q\ngot:\n%s", forbidden, out)
		}
	}
}

// TestVersionCommandOutput drives the actual Cobra command.
func TestVersionCommandOutput(t *testing.T) {
	var buf bytes.Buffer
	versionCmd.SetOut(&buf)
	t.Cleanup(func() { versionCmd.SetOut(nil) })

	versionCmd.Run(versionCmd, nil)

	out := buf.String()
	if !strings.Contains(out, "os/arch:") || !strings.Contains(out, "go:") {
		t.Errorf("version command output incomplete\ngot:\n%s", out)
	}
	if strings.Contains(out, "sting dev") {
		t.Errorf("version command reported the unstamped sentinel\ngot:\n%s", out)
	}
}

// TestResolvedIsSingleSource guards FR-005: the version command and the update
// path must read the same resolver, so they cannot disagree about what is
// running.
func TestResolvedIsSingleSource(t *testing.T) {
	if got := resolved(); got != buildinfo.Resolve(Version, Commit, Date) {
		t.Errorf("resolved() diverged from buildinfo.Resolve: %+v", got)
	}
}
