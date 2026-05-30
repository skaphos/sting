// SPDX-License-Identifier: MIT
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/skaphos/sting/internal/credentials"
	"github.com/spf13/cobra"
)

var authGitLabCmd = &cobra.Command{
	Use:     "gitlab",
	Aliases: []string{"login gitlab"},
	Short:   "Authenticate with GitLab",
	Long: `Authenticate with GitLab (GitLab.com or self-hosted).

Full OAuth device flow support (similar to glab) is coming soon.

For now you can:
  - Store a Personal Access Token using --with-token (recommended for the new credential system)
  - Continue using legacy PATs via STING_GITLAB_TOKEN or config

Examples:
  sting auth gitlab
  sting auth login gitlab
  sting auth gitlab --hostname gitlab.example.com --with-token < mytoken.txt`,
	RunE: runAuthGitLab,
}

var (
	authGitLabHostname string
	authGitLabWithToken bool
)

func init() {
	authGitLabCmd.Flags().StringVar(&authGitLabHostname, "hostname", "", "GitLab hostname (default: gitlab.com)")
	authGitLabCmd.Flags().BoolVar(&authGitLabWithToken, "with-token", false, "Read a Personal Access Token from standard input and store it")
}

func runAuthGitLab(cmd *cobra.Command, _ []string) error {
	hostname := authGitLabHostname
	if hostname == "" {
		hostname = "gitlab.com"
	}

	provider := credentials.ProviderGitLab

	if authGitLabWithToken {
		// Read PAT from stdin (like gh --with-token)
		scanner := bufio.NewScanner(os.Stdin)
		var token string
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				token = line
				break
			}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("failed to read token from stdin: %w", err)
		}
		if token == "" {
			return fmt.Errorf("no token provided on stdin")
		}

		store, err := credentials.New()
		if err != nil {
			return fmt.Errorf("initialize credential store: %w", err)
		}

		cred := credentials.Token{
			Type:        credentials.TokenTypePAT,
			AccessToken: token,
		}

		usedInsecure, err := store.Save(cmd.Context(), provider, hostname, cred, false)
		if err != nil {
			return fmt.Errorf("failed to store GitLab token: %w", err)
		}

		if usedInsecure {
			fmt.Fprintf(cmd.OutOrStdout(), "✓ GitLab token stored (insecure fallback).\n")
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "✓ GitLab token stored in system keyring.\n")
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  Host: %s\n", hostname)
		fmt.Fprintln(cmd.OutOrStdout(), "\nYou can now use `sting auth status` to verify.")
		return nil
	}

	// Default path: show guidance
	fmt.Fprintln(cmd.OutOrStdout(), "GitLab authentication")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintf(cmd.OutOrStdout(), "Target: %s\n\n", hostname)
	fmt.Fprintln(cmd.OutOrStdout(), "Full OAuth device flow (like glab) is coming soon.")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "For now you can store a Personal Access Token:")
	fmt.Fprintf(cmd.OutOrStdout(), "  sting auth gitlab --hostname %s --with-token < mytoken.txt\n", hostname)
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Or continue using the legacy method:")
	fmt.Fprintln(cmd.OutOrStdout(), "  STING_GITLAB_TOKEN or 'gitlab_token' in config")

	return nil
}
