// SPDX-License-Identifier: MIT
package cli

import (
	"github.com/spf13/cobra"
)

// authLoginCmd is the verbose form of the per-provider auth commands. It exists
// so that both `sting auth github` and `sting auth login github` (and the GitLab
// equivalents) work, per the SKA-467 design. The shorter `sting auth <provider>`
// form is the recommended one; `login` is offered for users who expect the more
// explicit `gh auth login` / `glab auth login` phrasing.
var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with a provider (verbose form of `sting auth <provider>`)",
	Long: "Verbose form of the per-provider auth commands.\n\n" +
		"  sting auth login github   is equivalent to   sting auth github\n" +
		"  sting auth login gitlab   is equivalent to   sting auth gitlab\n\n" +
		"Both forms are fully supported and share the same flags; the shorter\n" +
		"`sting auth <provider>` form is recommended.",
}

// The login subcommands reuse the exact RunE handlers and flag surface of the
// top-level `auth github` / `auth gitlab` commands, so the two spellings behave
// identically.
var authLoginGitHubCmd = &cobra.Command{
	Use:   "github",
	Short: "Authenticate with GitHub using OAuth (same as `sting auth github`)",
	Long: "Authenticate with GitHub using OAuth. This is the verbose form of\n" +
		"`sting auth github` and behaves identically. See `sting auth github --help`\n" +
		"for the full flag reference (--hostname, --web, --client-id, etc.).",
	RunE: runAuthGitHub,
}

var authLoginGitLabCmd = &cobra.Command{
	Use:   "gitlab",
	Short: "Authenticate with GitLab using OAuth (same as `sting auth gitlab`)",
	Long: "Authenticate with GitLab using OAuth device flow. This is the verbose\n" +
		"form of `sting auth gitlab` and behaves identically. See\n" +
		"`sting auth gitlab --help` for the full flag reference.",
	RunE: runAuthGitLab,
}

func init() {
	addAuthGitHubFlags(authLoginGitHubCmd)
	addAuthGitLabFlags(authLoginGitLabCmd)

	authLoginCmd.AddCommand(authLoginGitHubCmd)
	authLoginCmd.AddCommand(authLoginGitLabCmd)
}
