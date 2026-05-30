// SPDX-License-Identifier: MIT
package cli

import (
	"fmt"

	"github.com/skaphos/sting/internal/credentials"
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
	out := cmd.OutOrStdout()

	fmt.Fprintln(out, "Welcome to Sting!")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Let's get you set up.")

	// Check current authentication state (best effort)
	store, err := credentials.New()
	if err == nil {
		ghTok, _, _ := store.Load(cmd.Context(), credentials.ProviderGitHub, "github.com")
		glTok, _, _ := store.Load(cmd.Context(), credentials.ProviderGitLab, "gitlab.com")

		if ghTok.AccessToken != "" || glTok.AccessToken != "" {
			fmt.Fprintln(out)
			fmt.Fprintln(out, "You already have some credentials configured:")
			if ghTok.AccessToken != "" {
				fmt.Fprintln(out, "  ✓ GitHub")
			}
			if glTok.AccessToken != "" {
				fmt.Fprintln(out, "  ✓ GitLab")
			}
			fmt.Fprintln(out)
			fmt.Fprintln(out, "You're good to go! Try:")
			fmt.Fprintln(out, "  sting query --author YOUR_USERNAME")
			return nil
		}
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "No credentials found yet.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Which provider would you like to set up first?")
	fmt.Fprintln(out, "  1) GitHub (recommended for most people)")
	fmt.Fprintln(out, "  2) GitLab")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Run one of these commands to authenticate:")
	fmt.Fprintln(out, "  sting auth github")
	fmt.Fprintln(out, "  sting auth gitlab")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "After that, come back and run `sting init` again, or just try:")
	fmt.Fprintln(out, "  sting query --author YOUR_USERNAME")

	return nil
}
