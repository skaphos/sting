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

	fmt.Fprintln(cmd.OutOrStdout(), "Authentication status:")

	refs, err := store.List(cmd.Context())
	if err != nil {
		return fmt.Errorf("list credentials: %w", err)
	}

	hasNew := len(refs) > 0
	hasLegacy := false

	if hasNew {
		for _, ref := range refs {
			source := string(ref.Source)
			if ref.Username != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s/%s  (user: %s, source: %s)\n",
					ref.Provider, ref.Host, ref.Username, source)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s/%s  (source: %s)\n",
					ref.Provider, ref.Host, source)
			}
		}
	}

	// Report legacy sources for visibility during transition.
	if token := v.GetString("token"); token != "" {
		fmt.Fprintln(cmd.OutOrStdout(), "  github (legacy): token present via config or STING_TOKEN")
		hasLegacy = true
	}
	if token := v.GetString("gitlab_token"); token != "" {
		fmt.Fprintln(cmd.OutOrStdout(), "  gitlab (legacy): gitlab_token present via config or STING_GITLAB_TOKEN")
		hasLegacy = true
	}

	if !hasNew && !hasLegacy {
		fmt.Fprintln(cmd.OutOrStdout(), "  No credentials found.")
		fmt.Fprintln(cmd.OutOrStdout(), "  Run `sting auth github` or `sting auth gitlab` to authenticate.")
	}

	return nil
}
