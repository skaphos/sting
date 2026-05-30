// SPDX-License-Identifier: MIT
package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/skaphos/sting/internal/credentials"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	in := bufio.NewReader(os.Stdin)

	fmt.Fprintln(out, "Welcome to Sting!")
	fmt.Fprintln(out)

	// Check current state
	store, _ := credentials.New()
	var ghTok, glTok credentials.Token
	if store != nil {
		ghTok, _, _ = store.Load(cmd.Context(), credentials.ProviderGitHub, "github.com")
		glTok, _, _ = store.Load(cmd.Context(), credentials.ProviderGitLab, "gitlab.com")
	}

	if ghTok.AccessToken != "" {
		fmt.Fprintln(out, "✓ GitHub credentials found. Sting is ready to use (default provider = GitHub).")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Try:  sting query --author YOUR_GITHUB_HANDLE")
		if glTok.AccessToken != "" {
			fmt.Fprintln(out, "      (GitLab credentials also present)")
		}
		// Ensure default_provider is set to github
		ensureDefaultProvider("github")
		return nil
	}

	if glTok.AccessToken != "" {
		fmt.Fprintln(out, "GitLab credentials found, but no GitHub credentials.")
		fmt.Fprintln(out, "Sting defaults to GitHub.")
		fmt.Fprintln(out)
		ensureDefaultProvider("gitlab") // user is GitLab-heavy for now
		fmt.Fprintln(out, "Would you like to also set up GitHub now? [Y/n]")
		answer := prompt(in)
		if answer == "y" || answer == "" {
			return runGitHubAuthWizard(cmd, out, in)
		}
		fmt.Fprintln(out, "\nYou can set up GitHub later with: sting auth github")
		return nil
	}

	// Fresh user - GitHub first wizard
	fmt.Fprintln(out, "No credentials found yet.")
	fmt.Fprintln(out, "Sting defaults to GitHub.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Would you like to authenticate with GitHub now? [Y/n]")
	answer := prompt(in)

	if answer == "n" {
		fmt.Fprintln(out, "\nYou can run `sting auth gitlab` if you primarily use GitLab.")
		fmt.Fprintln(out, "Or come back later with `sting auth github`.")
		ensureDefaultProvider("gitlab")
		return nil
	}

	return runGitHubAuthWizard(cmd, out, in)
}

func prompt(in *bufio.Reader) string {
	fmt.Print("> ")
	text, _ := in.ReadString('\n')
	return strings.ToLower(strings.TrimSpace(text))
}

func runGitHubAuthWizard(cmd *cobra.Command, out io.Writer, in *bufio.Reader) error {
	fmt.Fprintln(out, "\nLaunching GitHub authentication (device flow)...")
	fmt.Fprintln(out, "This is the same as running: sting auth github")

	// Prepare for default github.com flow
	authGitHubHostname = ""
	authGitHubWeb = false
	authGitHubInsecure = false
	authGitHubClipboard = false
	authGitHubClientID = ""
	authGitHubClientSecret = ""

	if err := runAuthGitHub(cmd, nil); err != nil {
		return err
	}

	// After successful auth, set GitHub as the default provider
	ensureDefaultProvider("github")

	fmt.Fprintln(out, "\n✓ GitHub is now your default provider.")
	fmt.Fprintln(out, "Try:  sting query --author YOUR_GITHUB_HANDLE")
	return nil
}

// ensureDefaultProvider sets the provider in viper and attempts to persist it
// to the user's config file.
func ensureDefaultProvider(provider string) {
	v.Set("provider", provider)

	// Try to write the config. If no file exists yet, create one in the default location.
	if err := v.WriteConfig(); err != nil {
		// No config file yet — create it
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Find the first config dir we would have searched
			dirs := configSearchDirs()
			if len(dirs) > 0 {
				dir := dirs[0]
				if err := os.MkdirAll(dir, 0700); err == nil {
					path := filepath.Join(dir, "config.yaml")
					v.SetConfigFile(path)
					_ = v.WriteConfig()
				}
			}
		}
	}
}
