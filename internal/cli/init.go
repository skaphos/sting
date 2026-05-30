// SPDX-License-Identifier: MIT

//nolint:errcheck // This file is the interactive auth/init wizard; virtually all output is human-facing fmt.Fprint* to Cobra's OutOrStdout(). Write failures to stdout are not actionable in a CLI.
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
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Guided setup for first-time users",
	Long: `Guided first-time setup for Sting.

Sting defaults to GitHub. Running "sting init" (or "sting init github")
will help you authenticate with GitHub and set it as your default provider.

GitLab is fully supported as a secondary provider.

Examples:
  sting init
  sting init github
  sting init gitlab
`,
}

var initGitHubCmd = &cobra.Command{
	Use:   "github",
	Short: "Set up GitHub (the default)",
	RunE:  runInitGitHub,
}

var initGitLabCmd = &cobra.Command{
	Use:   "gitlab",
	Short: "Set up GitLab",
	RunE:  runInitGitLab,
}

var initYes bool

func init() {
	initCmd.AddCommand(initGitHubCmd)
	initCmd.AddCommand(initGitLabCmd)

	initCmd.PersistentFlags().BoolVarP(&initYes, "yes", "y", false, "Non-interactive mode (assume yes to defaults)")
}

func runInit(cmd *cobra.Command, _ []string) error {
	// Bare "sting init" defaults to GitHub
	return runInitGitHub(cmd, nil)
}

func runInitGitHub(cmd *cobra.Command, _ []string) error {
	return runProviderInit(cmd, credentials.ProviderGitHub)
}

func runInitGitLab(cmd *cobra.Command, _ []string) error {
	return runProviderInit(cmd, credentials.ProviderGitLab)
}

//nolint:errcheck // All fmt.Fprint* calls here are for human CLI output to cmd.OutOrStdout(); failure to write to stdout is not actionable in a CLI wizard.
func runProviderInit(cmd *cobra.Command, provider credentials.Provider) error {
	out := cmd.OutOrStdout()
	in := bufio.NewReader(os.Stdin)

	fmt.Fprintln(out, "Welcome to Sting!")
	fmt.Fprintln(out)

	store, _ := credentials.New()
	var ghTok, glTok credentials.Token
	if store != nil {
		ghTok, _, _ = store.Load(cmd.Context(), credentials.ProviderGitHub, "github.com")
		glTok, _, _ = store.Load(cmd.Context(), credentials.ProviderGitLab, "gitlab.com")
	}

	// If already have the target provider
	if provider == credentials.ProviderGitHub && ghTok.AccessToken != "" {
		fmt.Fprintln(out, "✓ GitHub credentials found. Sting is ready (GitHub is default).")
		fmt.Fprintln(out, "Try:  sting query --author YOUR_GITHUB_HANDLE")
		if glTok.AccessToken != "" {
			fmt.Fprintln(out, "      (GitLab credentials also present)")
		}
		ensureDefaultProvider("github")
		printFinalSummary(out)
		return offerInstall(cmd, out, in)
	}

	if provider == credentials.ProviderGitLab && glTok.AccessToken != "" {
		fmt.Fprintln(out, "✓ GitLab credentials found.")
		fmt.Fprintln(out, "Note: Sting defaults to GitHub. You can also set up GitHub with `sting init github`.")
		ensureDefaultProvider("gitlab")
		printFinalSummary(out)
		return offerInstall(cmd, out, in)
	}

	// Fresh setup for this provider
	if !initYes {
		fmt.Fprintln(out, "No credentials found yet for this provider.")
		fmt.Fprintln(out, "Would you like to authenticate now? [Y/n]")
		if prompt(in) == "n" {
			fmt.Fprintln(out, "\nYou can run it later with:")
			if provider == credentials.ProviderGitHub {
				fmt.Fprintln(out, "  sting auth github")
			} else {
				fmt.Fprintln(out, "  sting auth gitlab")
			}
			printFinalSummary(out)
			return nil
		}
	}

	// Proceed with auth wizard
	if provider == credentials.ProviderGitHub {
		return runGitHubAuthWizard(cmd, out, in)
	} else {
		return runGitLabAuthWizard(cmd, out, in)
	}
}

//nolint:errcheck
func printFinalSummary(out io.Writer) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "All set!")
	fmt.Fprintln(out, "Next steps:")
	fmt.Fprintln(out, "  sting query --author YOUR_USERNAME")
	fmt.Fprintln(out, "  sting install          # optional: register with your agents")
}

//nolint:errcheck
func prompt(in *bufio.Reader) string {
	fmt.Print("> ")
	text, _ := in.ReadString('\n')
	return strings.ToLower(strings.TrimSpace(text))
}

func runGitHubAuthWizard(cmd *cobra.Command, out io.Writer, in *bufio.Reader) error {
	fmt.Fprintln(out, "\nLaunching GitHub authentication (device flow)...")
	fmt.Fprintln(out, "This is the same as running: sting auth github")

	authGitHubHostname = ""
	authGitHubWeb = false
	authGitHubInsecure = false
	authGitHubClipboard = false
	authGitHubClientID = ""
	authGitHubClientSecret = ""

	if err := runAuthGitHub(cmd, nil); err != nil {
		fmt.Fprintln(out, "\nGitHub authentication was not completed.")
		fmt.Fprintln(out, "You can try again later with: sting auth github")
		ensureDefaultProvider("github")
		printFinalSummary(out)
		return nil // graceful - don't fail the whole init
	}

	// Re-check and give strong success message
	store, _ := credentials.New()
	if store != nil {
		tok, _, _ := store.Load(cmd.Context(), credentials.ProviderGitHub, "github.com")
		if tok.AccessToken != "" {
			fmt.Fprintln(out, "\n✓ GitHub authentication successful!")
			ensureDefaultProvider("github")
			fmt.Fprintln(out, "GitHub is now your default provider.")
			printFinalSummary(out)
			return offerInstall(cmd, out, in)
		}
	}

	ensureDefaultProvider("github")
	printFinalSummary(out)
	return offerInstall(cmd, out, in)
}

func runGitLabAuthWizard(cmd *cobra.Command, out io.Writer, in *bufio.Reader) error {
	fmt.Fprintln(out, "\nLaunching GitLab authentication (device flow)...")
	fmt.Fprintln(out, "This is the same as running: sting auth gitlab")

	// Prepare globals for GitLab device flow
	authGitLabHostname = ""
	authGitLabWithToken = false
	authGitLabClientID = ""
	authGitLabClientSecret = ""
	authGitLabClipboard = false
	authGitLabWeb = false
	authGitLabInsecure = false

	if err := runAuthGitLab(cmd, nil); err != nil {
		fmt.Fprintln(out, "\nGitLab authentication was not completed.")
		fmt.Fprintln(out, "You can try again later with: sting auth gitlab")
		ensureDefaultProvider("gitlab")
		printFinalSummary(out)
		return nil
	}

	// Re-check
	store, _ := credentials.New()
	if store != nil {
		tok, _, _ := store.Load(cmd.Context(), credentials.ProviderGitLab, "gitlab.com")
		if tok.AccessToken != "" {
			fmt.Fprintln(out, "\n✓ GitLab authentication successful!")
			ensureDefaultProvider("gitlab")
			fmt.Fprintln(out, "Note: GitHub is still the default. You can change it with `sting init github` if needed.")
			printFinalSummary(out)
			return offerInstall(cmd, out, in)
		}
	}

	ensureDefaultProvider("gitlab")
	printFinalSummary(out)
	return offerInstall(cmd, out, in)
}

// ensureDefaultProvider sets the provider in viper and reliably writes it
// to ~/.config/sting/config.yaml (creating directories and file as needed).
func ensureDefaultProvider(provider string) {
	v.Set("provider", provider)

	// Best effort: ensure we have a config file we can write to
	dirs := configSearchDirs()
	if len(dirs) > 0 {
		dir := dirs[0]
		_ = os.MkdirAll(dir, 0700)
		configPath := filepath.Join(dir, "config.yaml")
		v.SetConfigFile(configPath)

		// If the file doesn't exist, create a minimal one so WriteConfig succeeds
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			_ = os.WriteFile(configPath, []byte("# Sting configuration\n"), 0600)
		}
	}

	_ = v.WriteConfig()
}

// offerInstall asks the user if they want to register Sting with their agent runtimes.
func offerInstall(cmd *cobra.Command, out io.Writer, in *bufio.Reader) error {
	if initYes {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "To register Sting with your agents later, run:")
		fmt.Fprintln(out, "  sting install")
		return nil
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Would you like to register Sting with your agent runtimes now? (Claude, Codex, etc.) [Y/n]")
	answer := prompt(in)

	if answer == "n" {
		fmt.Fprintln(out, "\nYou can do this later with: sting install")
		return nil
	}

	fmt.Fprintln(out, "\nLaunching installer...")
	return runInstall(cmd, nil)
}
