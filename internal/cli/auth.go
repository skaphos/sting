// SPDX-License-Identifier: MIT
package cli

import (
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with GitHub or GitLab",
	Long: "Manage authentication for GitHub and GitLab.\n\n" +
		"Use explicit subcommands to log in to a specific provider:\n" +
		"  sting auth github\n" +
		"  sting auth gitlab\n\n" +
		"Other commands:\n" +
		"  sting auth status     Show current authentication state\n" +
		"  sting auth logout     Remove stored credentials",
}

func init() {
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authGitHubCmd)
	authCmd.AddCommand(authLogoutCmd)
	// TODO (GitLab later): authCmd.AddCommand(authGitLabCmd)
}
