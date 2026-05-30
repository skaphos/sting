// SPDX-License-Identifier: MIT
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

This removes the token from the secure keyring (preferred) and any
insecure fallback storage.

Examples:
  sting auth logout
  sting auth logout github
  sting auth logout gitlab`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAuthLogout,
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

	store, err := credentials.New()
	if err != nil {
		return fmt.Errorf("initialize credential store: %w", err)
	}

	if err := store.Delete(cmd.Context(), provider, host); err != nil {
		return fmt.Errorf("failed to remove credentials for %s: %w", provider, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "✓ Logged out of %s\n", provider)
	return nil
}
