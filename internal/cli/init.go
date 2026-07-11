// SPDX-License-Identifier: MIT

//nolint:errcheck // This file is the interactive auth/init wizard; virtually all output is human-facing fmt.Fprint* to Cobra's OutOrStdout(). Write failures to stdout are not actionable in a CLI.
package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/skaphos/sting/internal/credentials"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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

	initCmd.RunE = runInit
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
	in := bufio.NewReader(cmd.InOrStdin())

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
		if prompt(out, in) == "n" {
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
func prompt(out io.Writer, in *bufio.Reader) string {
	fmt.Fprint(out, "> ")
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
			fmt.Fprintln(out, "Note: GitLab is now the default. You can change it with `sting init github` if needed.")
			printFinalSummary(out)
			return offerInstall(cmd, out, in)
		}
	}

	ensureDefaultProvider("gitlab")
	printFinalSummary(out)
	return offerInstall(cmd, out, in)
}

// ensureDefaultProvider sets the provider in viper (for the rest of this
// process) and persists it to ~/.config/sting/config.yaml (creating
// directories and file as needed).
//
// It deliberately does NOT use viper's WriteConfig: WriteConfig serializes
// viper's entire merged state (defaults + env + bound flags), which would
// write any token/gitlab_token sourced from STING_TOKEN/STING_GITLAB_TOKEN
// to disk in cleartext, and would rewrite the whole file, destroying any
// comments or formatting the user had. Instead this does a targeted
// read-modify-write of just the affected keys via setConfigKeys, so the rest
// of the file (including secrets that must never be written here) is left
// untouched.
func ensureDefaultProvider(provider string) {
	v.Set("provider", provider)

	dirs := configSearchDirs()
	if len(dirs) == 0 {
		return
	}
	dir := dirs[0]
	if err := os.MkdirAll(dir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "sting: warning: could not create config directory %s: %v\n", dir, err)
		return
	}
	configPath := filepath.Join(dir, "config.yaml")

	keys := map[string]string{"provider": provider}
	if provider == "gitlab" {
		// GitLab doesn't support the built-in default_scope of "search" (see
		// config.Config.Validate): every bare query would otherwise fail
		// validation. Pin a scope GitLab does support so the config sting
		// just wrote is actually usable.
		keys["default_scope"] = "repos"
		v.Set("default_scope", "repos")
	}

	if err := setConfigKeys(configPath, keys); err != nil {
		// Do not swallow this: the user's default-provider choice silently
		// failing to persist is worth surfacing, even though init otherwise
		// tolerates failures gracefully.
		fmt.Fprintf(os.Stderr, "sting: warning: could not persist config to %s: %v\n", configPath, err)
		return
	}

	v.SetConfigFile(configPath)
}

// setConfigKeys does a targeted read-modify-write of the given top-level
// string keys in the YAML file at path, leaving every other key, comment,
// and formatting choice untouched. It creates the file (and a minimal
// top-level mapping) if it does not yet exist.
//
// Callers must never pass secret keys (token, gitlab_token, ...) here: the
// whole point is to avoid ever writing sting's config file from viper's
// merged state, which is how env-sourced tokens leak into it.
func setConfigKeys(path string, keys map[string]string) error {
	var doc yaml.Node
	isNew := true

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if len(bytes.TrimSpace(data)) > 0 {
			if uerr := yaml.Unmarshal(data, &doc); uerr != nil {
				return fmt.Errorf("parse %s: %w", path, uerr)
			}
			isNew = false
		}
	case os.IsNotExist(err):
		// Fall through: build a fresh document below.
	default:
		return fmt.Errorf("read %s: %w", path, err)
	}

	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode}
	}

	var mapping *yaml.Node
	if len(doc.Content) == 0 {
		mapping = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		if isNew {
			mapping.HeadComment = "Sting configuration"
		}
		doc.Content = []*yaml.Node{mapping}
	} else {
		mapping = doc.Content[0]
		if mapping.Kind != yaml.MappingNode {
			return fmt.Errorf("%s: top-level YAML value is not a mapping", path)
		}
	}

	for key, val := range keys {
		setMappingString(mapping, key, val)
	}

	out, merr := yaml.Marshal(&doc)
	if merr != nil {
		return fmt.Errorf("encode %s: %w", path, merr)
	}
	if werr := os.WriteFile(path, out, 0600); werr != nil {
		return fmt.Errorf("write %s: %w", path, werr)
	}
	return nil
}

// setMappingString sets key to val in a YAML mapping node, updating the
// existing value node in place (preserving any comments attached to it) if
// the key is already present, or appending a new key/value pair otherwise.
func setMappingString(mapping *yaml.Node, key, val string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			value := mapping.Content[i+1]
			value.Kind = yaml.ScalarNode
			value.Tag = "!!str"
			value.Style = 0
			value.Value = val
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: val},
	)
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
	answer := prompt(out, in)

	if answer == "n" {
		fmt.Fprintln(out, "\nYou can do this later with: sting install")
		return nil
	}

	fmt.Fprintln(out, "\nLaunching installer...")
	return runInstall(cmd, nil)
}
