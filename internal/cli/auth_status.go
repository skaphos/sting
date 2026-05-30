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
		"Use --hostname to check a specific GitHub Enterprise Server or GitLab instance.\n\n" +
		"This command is read-only and safe to run at any time.",
	Args: cobra.NoArgs,
	RunE: runAuthStatus,
}

var authStatusHostname string

func init() {
	authStatusCmd.Flags().StringVar(&authStatusHostname, "hostname", "", "Check status for a specific hostname (e.g. a GitHub Enterprise Server instance)")
}

func runAuthStatus(cmd *cobra.Command, _ []string) error {
	store, err := credentials.New()
	if err != nil {
		return fmt.Errorf("initialize credential store: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Authentication status:")
	fmt.Fprintln(cmd.OutOrStdout())

	refs, err := store.List(cmd.Context())
	if err != nil {
		return fmt.Errorf("list credentials: %w", err)
	}

	// Filter by hostname if specified (important for GHES)
	if authStatusHostname != "" {
		filtered := []credentials.CredentialRef{}
		for _, r := range refs {
			if r.Host == authStatusHostname {
				filtered = append(filtered, r)
			}
		}
		refs = filtered
	}

	// Separate GitHub hosts (including GHES) and GitLab
	githubHosts := map[string][]credentials.CredentialRef{}
	gitlabHosts := []credentials.CredentialRef{}

	for _, ref := range refs {
		if ref.Provider == credentials.ProviderGitHub {
			githubHosts[ref.Host] = append(githubHosts[ref.Host], ref)
		} else {
			gitlabHosts = append(gitlabHosts, ref)
		}
	}

	// GitHub section (including GHES)
	fmt.Fprintln(cmd.OutOrStdout(), "GitHub:")
	if len(githubHosts) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "  Not logged in.")
	} else {
		for host, entries := range githubHosts {
			for _, ref := range entries {
				source := string(ref.Source)
				if ref.Username != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  ✓ Logged into %s as %s (source: %s)\n", host, ref.Username, source)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "  ✓ Logged into %s (source: %s)\n", host, source)
				}
			}
		}
	}

	// Legacy GitHub (global, only show when not filtering to a specific host)
	if authStatusHostname == "" {
		if token := v.GetString("token"); token != "" {
			fmt.Fprintln(cmd.OutOrStdout(), "  • Legacy token available via STING_TOKEN / config (fallback)")
		}
	}

	fmt.Fprintln(cmd.OutOrStdout())

	// GitLab section
	fmt.Fprintln(cmd.OutOrStdout(), "GitLab:")
	if len(gitlabHosts) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "  Not logged in.")
	} else {
		for _, ref := range gitlabHosts {
			source := string(ref.Source)
			if ref.Username != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  ✓ Logged into %s as %s (source: %s)\n", ref.Host, ref.Username, source)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "  ✓ Logged into %s (source: %s)\n", ref.Host, source)
			}
		}
	}

	if authStatusHostname == "" {
		if token := v.GetString("gitlab_token"); token != "" {
			fmt.Fprintln(cmd.OutOrStdout(), "  • Legacy token available via STING_GITLAB_TOKEN / config (fallback)")
		}
	}

	fmt.Fprintln(cmd.OutOrStdout())
	if authStatusHostname != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Run `sting auth github --hostname %s` to authenticate.\n", authStatusHostname)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Run `sting auth github` to authenticate with GitHub using OAuth (recommended).")
		fmt.Fprintln(cmd.OutOrStdout(), "Use --hostname for GitHub Enterprise Server or self-hosted GitLab instances.")
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Legacy PATs continue to work as a fallback.")

	return nil
}
