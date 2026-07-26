// SPDX-License-Identifier: MIT
package cli

import (
	"fmt"
	"io"
	"runtime"

	"github.com/skaphos/sting/internal/buildinfo"
	"github.com/spf13/cobra"
)

// Build metadata, set via -ldflags at release time by goreleaser. These remain
// the stamping target named in .goreleaser.yaml; do not rename them.
//
// The defaults are sentinels, not values. A binary carrying them was not
// stamped, and buildinfo falls through to the metadata the Go toolchain records
// so that `go build` and `go install` still report honestly.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// resolved returns the running binary's identity, as `sting version` reports
// it. sting does not update itself (ADR 0011), so this is the value a user
// compares against the latest release to decide whether to upgrade.
func resolved() buildinfo.Info {
	return buildinfo.Resolve(Version, Commit, Date)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version and build information",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		writeVersion(cmd.OutOrStdout(), resolved())
	},
}

// writeVersion renders build information. Unrecorded fields are reported as
// unavailable rather than filled with a placeholder that could be mistaken for
// a real value.
func writeVersion(w io.Writer, info buildinfo.Info) {
	if info.Known() {
		fmt.Fprintf(w, "sting %s\n", info.Version)
	} else {
		fmt.Fprint(w, "sting (version unavailable)\n")
	}

	fmt.Fprintf(w, "  commit:  %s\n", commitField(info))
	fmt.Fprintf(w, "  built:   %s\n", orUnknown(info.Time))
	fmt.Fprintf(w, "  go:      %s\n", runtime.Version())
	fmt.Fprintf(w, "  os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

// commitField renders the revision, marking a build made from a working tree
// with uncommitted changes so it is not mistaken for a clean build of that
// revision.
func commitField(info buildinfo.Info) string {
	rev := orUnknown(info.Revision)
	if info.Modified {
		return rev + " (modified)"
	}
	return rev
}

func orUnknown(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}
