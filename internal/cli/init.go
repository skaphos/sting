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
	Long: `Guided first-time setup for Sting.

Sting defaults to GitHub. This command will help you get authenticated
and ready to query commits.

Primary flow:
  sting init
  sting auth github     # the recommended default

GitLab is fully supported but treated as a secondary/optional provider.

You can always run the auth commands directly:
  sting auth github
  sting auth gitlab
`,
	Args: cobra.NoArgs,
	RunE: runInit,
}

func runInit(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	fmt.Fprintln(out, "Welcome to Sting!")
	fmt.Fprintln(out)

	// Check current authentication state (best effort)
	store, err := credentials.New()
	if err == nil {
		ghTok, _, _ := store.Load(cmd.Context(), credentials.ProviderGitHub, "github.com")
		glTok, _, _ := store.Load(cmd.Context(), credentials.ProviderGitLab, "gitlab.com")

		if ghTok.AccessToken != "" {
			fmt.Fprintln(out, "GitHub credentials detected. You're ready to go!")
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Try:")
			fmt.Fprintln(out, "  sting query --author YOUR_GITHUB_HANDLE")
			if glTok.AccessToken != "" {
				fmt.Fprintln(out, "  (GitLab credentials also present)")
			} else {
				fmt.Fprintln(out)
				fmt.Fprintln(out, "If you also want to query GitLab commits, run:")
				fmt.Fprintln(out, "  sting auth gitlab")
			}
			return nil
		}

		if glTok.AccessToken != "" {
			fmt.Fprintln(out, "GitLab credentials detected (no GitHub credentials found).")
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Note: Sting defaults to GitHub. If you'd like to use GitHub as well:")
			fmt.Fprintln(out, "  sting auth github")
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Or query GitLab directly:")
			fmt.Fprintln(out, "  sting query --provider gitlab --author YOUR_GITLAB_HANDLE")
			return nil
		}
	}

	// No credentials at all → GitHub-first guidance
	fmt.Fprintln(out, "No credentials found yet.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Sting defaults to GitHub. Let's get you set up:")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  sting auth github")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "This is the recommended path for most users.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "After authenticating, try:")
	fmt.Fprintln(out, "  sting query --author YOUR_GITHUB_HANDLE")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "GitLab is also supported if needed:")
	fmt.Fprintln(out, "  sting auth gitlab")

	return nil
}
