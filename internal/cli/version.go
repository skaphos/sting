// SPDX-License-Identifier: MIT
package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Build metadata, set via -ldflags at release time by goreleaser. The defaults
// apply to `go build`/`go install` and local development.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version and build information",
	Args:  cobra.NoArgs,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("sting %s\n", Version)
		fmt.Printf("  commit:  %s\n", Commit)
		fmt.Printf("  built:   %s\n", Date)
		fmt.Printf("  go:      %s\n", runtime.Version())
		fmt.Printf("  os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}
