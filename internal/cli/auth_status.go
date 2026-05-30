// SPDX-License-Identifier: MIT
package cli

import (
	"fmt"

	"github.com/skaphos/sting/internal/credentials"
	"github.com/spf13/cobra"
)

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status for GitHub and GitLab",
	Long: "Displays which providers/hosts have stored credentials, their source " +
		"(keyring, file, environment, or legacy config), and basic details.\n\n" +
		"This command is read-only and safe to run at any time.",
	Args: cobra.NoArgs,
	RunE: runAuthStatus,
}

func runAuthStatus(cmd *cobra.Command, _ []string) error {
	store, err := credentials.New()
	if err != nil {
		return fmt.Errorf("initialize credential store: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Authentication status:\n")

	refs, err := store.List(cmd.Context())
	if err != nil {
		return fmt.Errorf("list credentials: %w", err)
	}

	// Group by provider for nicer output
	githubEntries := []credentials.CredentialRef{}
	gitlabEntries := []credentials.CredentialRef{}

	for _, ref := range refs {
		switch ref.Provider {
		case credentials.ProviderGitHub:
			githubEntries = append(githubEntries, ref)
		case credentials.ProviderGitLab:
			gitlabEntries = append(gitlabEntries, ref)
		}
	}

	// GitHub section (focus area)
	fmt.Fprintln(cmd.OutOrStdout(), "GitHub:")
	if len(githubEntries) > 0 {
		for _, ref := range githubEntries {
			source := string(ref.Source)
			if ref.Username != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  ✓ Logged into %s as %s (source: %s)\n",
					ref.Host, ref.Username, source)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "  ✓ Logged into %s (source: %s)\n",
					ref.Host, source)
			}
		}
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "  Not logged in.")
	}

	// Legacy GitHub token (still supported as fallback)
	if token := v.GetString("token"); token != "" {
		fmt.Fprintln(cmd.OutOrStdout(), "  • Legacy token available via STING_TOKEN / config (fallback)")
	}

	fmt.Fprintln(cmd.OutOrStdout())

	// GitLab section (deprioritized for now)
	fmt.Fprintln(cmd.OutOrStdout(), "GitLab:")
	if len(gitlabEntries) > 0 {
		for _, ref := range gitlabEntries {
			source := string(ref.Source)
			if ref.Username != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  ✓ Logged into %s as %s (source: %s)\n",
					ref.Host, ref.Username, source)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "  ✓ Logged into %s (source: %s)\n",
					ref.Host, source)
			}
		}
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "  Not logged in.")
	}

	if token := v.GetString("gitlab_token"); token != "" {
		fmt.Fprintln(cmd.OutOrStdout(), "  • Legacy token available via STING_GITLAB_TOKEN / config (fallback)")
	}

	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Run `sting auth github` to authenticate with GitHub using OAuth (recommended).")
	fmt.Fprintln(cmd.OutOrStdout(), "Legacy PATs continue to work as a fallback.")

	return nil
}
