// SPDX-License-Identifier: MIT
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var authGitHubCmd = &cobra.Command{
	Use:     "github",
	Aliases: []string{"login github"},
	Short:   "Authenticate with GitHub using OAuth",
	Long: `Start the OAuth flow to authenticate with GitHub (recommended).

This is the primary way to authenticate with GitHub going forward.

Examples:
  sting auth github
  sting auth login github`,
	RunE: runAuthGitHub,
}

func runAuthGitHub(cmd *cobra.Command, _ []string) error {
	fmt.Fprintln(cmd.OutOrStdout(), "GitHub OAuth login is not yet implemented.")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "In the meantime you can continue using legacy PATs:")
	fmt.Fprintln(cmd.OutOrStdout(), "  - Set STING_TOKEN environment variable, or")
	fmt.Fprintln(cmd.OutOrStdout(), "  - Add 'token' to your sting config file.")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Full GitHub OAuth support (device + web flow) is coming next.")

	// TODO: Implement actual flow using github.com/cli/oauth
	// - Device flow by default (like gh)
	// - --web flag to force browser flow
	// - --hostname for GHES
	// - Store result via credentials.Store
	// - Support bring-your-own client ID/secret for advanced GHES cases

	return nil
}
