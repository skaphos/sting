// SPDX-License-Identifier: MIT
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Guided setup for first-time users",
	Long: `Run an interactive wizard to get started with Sting.

This will help you:
  - Choose your primary provider (GitHub or GitLab)
  - Authenticate using the recommended OAuth flow
  - Set a few sensible defaults

You can run individual steps manually at any time:
  sting auth github
  sting auth gitlab
  sting query --help
`,
	Args: cobra.NoArgs,
	RunE: runInit,
}

func runInit(cmd *cobra.Command, _ []string) error {
	fmt.Fprintln(cmd.OutOrStdout(), "Welcome to Sting!")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "This wizard will help you get set up.")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Recommended next step:")
	fmt.Fprintln(cmd.OutOrStdout(), "  sting auth github     # or: sting auth gitlab")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "After authenticating, try:")
	fmt.Fprintln(cmd.OutOrStdout(), "  sting query --author YOUR_GITHUB_HANDLE")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Run `sting --help` or `sting init --help` anytime.")

	return nil
}
