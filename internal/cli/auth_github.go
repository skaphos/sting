// SPDX-License-Identifier: MIT
package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/browser"
	"github.com/cli/oauth"
	"github.com/skaphos/sting/internal/credentials"
	"github.com/spf13/cobra"
)

var authGitHubCmd = &cobra.Command{
	Use:   "github",
	Short: "Authenticate with GitHub using OAuth",
	Long: `Start the OAuth flow to authenticate with GitHub (recommended).

Official Skaphos/Sting OAuth App credentials are included, so this works out of the box
for github.com.

GitHub Enterprise Server (GHES):
  Use --hostname to target your enterprise instance.
  You will almost always need to register your own OAuth App on that GHES instance
  and pass the credentials using --client-id / --client-secret (or the corresponding
  environment variables).

  Example:
    sting auth github --hostname ghe.example.com \
      --client-id YOUR_GHES_CLIENT_ID \
      --client-secret YOUR_GHES_CLIENT_SECRET

By default uses the device authorization flow (no browser required on the machine running sting).
Use --web to force the browser-based flow instead.

Examples:
  sting auth github
  sting auth login github
  sting auth github --hostname ghe.example.com --web`,
	RunE: runAuthGitHub,
}

var (
	authGitHubHostname     string
	authGitHubWeb          bool
	authGitHubInsecure     bool
	authGitHubClipboard    bool
	authGitHubClientID     string
	authGitHubClientSecret string
)

func init() {
	authGitHubCmd.Flags().StringVar(&authGitHubHostname, "hostname", "", "GitHub hostname to authenticate with (default: github.com)")
	authGitHubCmd.Flags().BoolVarP(&authGitHubWeb, "web", "w", false, "Open a browser to authenticate instead of using the device flow")
	authGitHubCmd.Flags().BoolVar(&authGitHubInsecure, "insecure-storage", false, "Save the token to the config file instead of the system keyring")
	authGitHubCmd.Flags().BoolVarP(&authGitHubClipboard, "clipboard", "c", false, "Copy the one-time code to the clipboard (device flow only)")

	// Allow overriding the OAuth app credentials (useful for GHES or bring-your-own)
	authGitHubCmd.Flags().StringVar(&authGitHubClientID, "client-id", "", "OAuth client ID (advanced)")
	authGitHubCmd.Flags().StringVar(&authGitHubClientSecret, "client-secret", "", "OAuth client secret (advanced)")
	_ = authGitHubCmd.Flags().MarkHidden("client-id")
	_ = authGitHubCmd.Flags().MarkHidden("client-secret")
}

//nolint:errcheck // fmt.Fprint* calls are for human CLI output; stdout write failures are not actionable here.
func runAuthGitHub(cmd *cobra.Command, _ []string) error {
	hostname := authGitHubHostname
	if hostname == "" {
		hostname = "github.com"
	}

	clientID := authGitHubClientID
	clientSecret := authGitHubClientSecret

	// Official Skaphos/Sting OAuth App credentials (safe to embed for a public client / CLI tool).
	// These can be overridden with --client-id / --client-secret or the environment variables
	// STING_GITHUB_CLIENT_ID / STING_GITHUB_CLIENT_SECRET (useful for GHES or bring-your-own apps).
	if clientID == "" {
		clientID = os.Getenv("STING_GITHUB_CLIENT_ID")
	}
	if clientID == "" {
		clientID = "Ov23liDHsFVqZE2z7r16"
	}

	if clientSecret == "" {
		clientSecret = os.Getenv("STING_GITHUB_CLIENT_SECRET")
	}
	if clientSecret == "" {
		clientSecret = "6b0e3062797258cdc9fcc80ce5b7774be2d4d0a2"
	}

	// GHES guidance: if the user is targeting a non-github.com host and is still
	// using the default public credentials, give a clear, actionable error.
	isEnterprise := hostname != "github.com" && !strings.HasSuffix(hostname, ".github.com")
	usingDefaultCreds := (authGitHubClientID == "" && os.Getenv("STING_GITHUB_CLIENT_ID") == "") &&
		(authGitHubClientSecret == "" && os.Getenv("STING_GITHUB_CLIENT_SECRET") == "")

	if isEnterprise && usingDefaultCreds {
		//lint:ignore ST1005 user-facing CLI error with proper punctuation and newlines
		//nolint:staticcheck // ST1005
		return fmt.Errorf(`GitHub Enterprise Server detected (%s) — built-in Skaphos credentials only work against github.com.

You need to register an OAuth App on your GHES instance and provide its credentials:

  sting auth github --hostname %s \
    --client-id YOUR_CLIENT_ID \
    --client-secret YOUR_CLIENT_SECRET

See the documentation for the exact settings (enable Device Flow, callback http://127.0.0.1/callback).`,
			hostname, hostname)
	}

	// Set up the OAuth flow (same library gh uses)
	host, err := oauth.NewGitHubHost("https://" + hostname)
	if err != nil {
		return fmt.Errorf("invalid GitHub host: %w", err)
	}

	flow := &oauth.Flow{
		Host:         host,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		CallbackURI:  "http://127.0.0.1/callback",
		Scopes:       []string{"repo", "read:org", "gist"}, // reasonable minimum for sting
	}

	// Customize the UX callbacks (modeled on gh's experience)
	flow.DisplayCode = func(code, verificationURL string) error {
		fmt.Fprintf(cmd.OutOrStdout(), "First copy your one-time code: %s\n", code)
		if authGitHubClipboard {
			if err := clipboard.WriteAll(code); err == nil {
				fmt.Fprintln(cmd.OutOrStdout(), "  (copied to clipboard)")
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Open %s in your browser to authorize.\n", verificationURL)
		return nil
	}

	flow.BrowseURL = func(url string) error {
		b := browser.New("", cmd.OutOrStdout(), cmd.ErrOrStderr())
		if err := b.Browse(url); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Failed to open browser: %v\n", err)
			fmt.Fprintf(cmd.OutOrStdout(), "Please open this URL manually: %s\n", url)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Opened %s in your browser.\n", url)
		}
		return nil
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Authenticating with GitHub...")

	token, err := flow.DetectFlow()
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Fetch the username using the token (GraphQL via go-gh)
	username := ""
	if client, err := api.NewGraphQLClient(api.ClientOptions{
		AuthToken: token.Token,
		Host:      hostname,
	}); err == nil {
		var query struct {
			Viewer struct {
				Login string
			}
		}
		if err := client.Query("UserCurrent", &query, nil); err == nil {
			username = query.Viewer.Login
		}
	}

	// Save the credential
	credStore, err := credentials.New()
	if err != nil {
		return fmt.Errorf("initialize credential store: %w", err)
	}

	cred := credentials.Token{
		Type:        credentials.TokenTypeOAuth,
		AccessToken: token.Token,
		Username:    username,
	}

	usedInsecure, err := credStore.Save(cmd.Context(), credentials.ProviderGitHub, hostname, cred, !authGitHubInsecure)
	if err != nil {
		return fmt.Errorf("failed to save credential: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout())
	if usedInsecure {
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Authentication complete. Token saved to config file (insecure).\n")
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "✓ Authentication complete. Token saved to system keyring.\n")
	}

	if username != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "  Logged in as %s on %s\n", username, hostname)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "  Logged into %s\n", hostname)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "\nYou can now use `sting auth status` to verify.")

	return nil
}
