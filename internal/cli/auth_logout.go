// SPDX-License-Identifier: MIT

//nolint:errcheck // CLI logout output uses fmt.Fprint* to Cobra streams; write errors are not actionable.
package cli

import (
	"fmt"
	"strings"

	"github.com/skaphos/sting/internal/credentials"
	"github.com/spf13/cobra"
)

var authLogoutCmd = &cobra.Command{
	Use:   "logout [github|gitlab]",
	Short: "Log out of a provider and remove stored credentials",
	Long: `Remove stored authentication credentials for a provider.

Use --hostname to log out of a specific GitHub Enterprise Server or GitLab instance.

This removes the token from the secure keyring (preferred) and any
insecure fallback storage.

Examples:
  sting auth logout
  sting auth logout github
  sting auth logout gitlab
  sting auth logout github --hostname ghe.example.com`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAuthLogout,
}

var authLogoutHostname string

func init() {
	authLogoutCmd.Flags().StringVar(&authLogoutHostname, "hostname", "", "Log out of a specific hostname (for GitHub Enterprise Server or self-hosted GitLab)")
}

func runAuthLogout(cmd *cobra.Command, args []string) error {
	providerArg := ""
	if len(args) > 0 {
		providerArg = strings.ToLower(args[0])
	}

	var provider credentials.Provider
	host := "github.com"

	switch providerArg {
	case "", "github":
		provider = credentials.ProviderGitHub
	case "gitlab":
		provider = credentials.ProviderGitLab
		host = "gitlab.com"
	default:
		return fmt.Errorf("unknown provider %q (use github or gitlab)", providerArg)
	}

	// Allow overriding the host for GHES / self-hosted instances
	if authLogoutHostname != "" {
		host = authLogoutHostname
	}

	store, err := credentials.New()
	if err != nil {
		return fmt.Errorf("initialize credential store: %w", err)
	}

	if err := store.Delete(cmd.Context(), provider, host); err != nil {
		return fmt.Errorf("failed to remove credentials for %s on %s: %w", provider, host, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "✓ Logged out of %s on %s\n", provider, host)
	return nil
}
