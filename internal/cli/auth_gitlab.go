// SPDX-License-Identifier: MIT
package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/cli/go-gh/v2/pkg/browser"
	"github.com/cli/oauth/device"
	"github.com/skaphos/sting/internal/credentials"
	"github.com/spf13/cobra"
)

var authGitLabCmd = &cobra.Command{
	Use:     "gitlab",
	Aliases: []string{"login gitlab"},
	Short:   "Authenticate with GitLab using OAuth Device Flow",
	Long: `Authenticate with GitLab.com or a self-hosted GitLab instance using the
OAuth 2.0 Device Authorization flow (the recommended method, same as glab).

You must provide a Client ID via --client-id or the STING_GITLAB_CLIENT_ID
environment variable. For self-hosted GitLab you will need to create your own
OAuth application and enable "Device authorization grant flow".

By default uses the device flow (no browser required on this machine).
Use --web to automatically open the verification URL in your browser.
Use --clipboard to copy the one-time code.

Examples:
  sting auth gitlab
  sting auth login gitlab --hostname gitlab.example.com --client-id YOUR_ID
  sting auth gitlab --web --client-id YOUR_ID
  echo 'glpat-xxxx' | sting auth gitlab --with-token`,
	RunE: runAuthGitLab,
}

var (
	authGitLabHostname     string
	authGitLabWithToken    bool
	authGitLabClientID     string
	authGitLabClientSecret string
	authGitLabClipboard    bool
	authGitLabWeb          bool
	authGitLabInsecure     bool
)

func init() {
	authGitLabCmd.Flags().StringVar(&authGitLabHostname, "hostname", "", "GitLab hostname (default: gitlab.com)")
	authGitLabCmd.Flags().BoolVar(&authGitLabWithToken, "with-token", false, "Read a Personal Access Token from standard input")
	authGitLabCmd.Flags().StringVar(&authGitLabClientID, "client-id", "", "OAuth application Client ID (required for device flow)")
	authGitLabCmd.Flags().StringVar(&authGitLabClientSecret, "client-secret", "", "OAuth application Client Secret (usually not needed)")
	authGitLabCmd.Flags().BoolVarP(&authGitLabClipboard, "clipboard", "c", false, "Copy the user code to the clipboard")
	authGitLabCmd.Flags().BoolVarP(&authGitLabWeb, "web", "w", false, "Open the verification URL in your browser automatically")
	authGitLabCmd.Flags().BoolVar(&authGitLabInsecure, "insecure-storage", false, "Save the token to the config file instead of the system keyring")
}

func runAuthGitLab(cmd *cobra.Command, _ []string) error {
	hostname := authGitLabHostname
	if hostname == "" {
		hostname = "gitlab.com"
	}

	provider := credentials.ProviderGitLab
	secureOnly := !authGitLabInsecure

	if authGitLabWithToken {
		scanner := bufio.NewScanner(cmd.InOrStdin())
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

		usedInsecure, err := store.Save(cmd.Context(), provider, hostname, cred, secureOnly)
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

	// Real Device Flow
	clientID := authGitLabClientID
	if clientID == "" {
		clientID = os.Getenv("STING_GITLAB_CLIENT_ID")
	}
	if clientID == "" {
		return fmt.Errorf(`GitLab device flow requires a client_id.

Create an OAuth application at:
  https://%s/-/user_settings/applications   (or on your self-hosted instance)

Enable "Device authorization grant flow".

Then run:
  sting auth gitlab --hostname %s --client-id YOUR_CLIENT_ID

You can also store a PAT with --with-token.`, hostname, hostname)
	}

	clientSecret := authGitLabClientSecret
	if clientSecret == "" {
		clientSecret = os.Getenv("STING_GITLAB_CLIENT_SECRET")
	}

	baseURL := "https://" + hostname
	deviceURL := baseURL + "/oauth/authorize_device"
	tokenURL := baseURL + "/oauth/token"

	fmt.Fprintf(cmd.OutOrStdout(), "Requesting device code for %s...\n", hostname)

	code, err := device.RequestCode(http.DefaultClient, deviceURL, clientID, []string{"read_api"})
	if err != nil {
		if err == device.ErrUnsupported {
			return fmt.Errorf("this GitLab instance does not support device flow. Use --with-token instead.")
		}
		return fmt.Errorf("failed to request device code: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nFirst copy your one-time code: %s\n", code.UserCode)

	if authGitLabClipboard {
		if err := clipboard.WriteAll(code.UserCode); err == nil {
			fmt.Fprintln(cmd.OutOrStdout(), "  (copied to clipboard)")
		}
	}

	verificationURI := code.VerificationURI
	if authGitLabWeb {
		b := browser.New("", cmd.OutOrStdout(), cmd.ErrOrStderr())
		if err := b.Browse(verificationURI); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Failed to open browser: %v\n", err)
			fmt.Fprintf(cmd.OutOrStdout(), "Please open this URL manually: %s\n", verificationURI)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Opened %s in your browser.\n", verificationURI)
		}
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Then visit: %s\n\n", verificationURI)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Waiting for authorization...")

	// Use the command context so the user can cancel the wait (Ctrl-C).
	tok, err := device.Wait(cmd.Context(), http.DefaultClient, tokenURL, device.WaitOptions{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		DeviceCode:   code,
	})
	if err != nil {
		if err == device.ErrTimeout {
			return fmt.Errorf("device authorization timed out")
		}
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Fetch username via GitLab REST (best effort; used for status display).
	username := fetchGitLabUsername(baseURL, tok.Token)

	// Save the OAuth token (mirrors GitHub flow UX and storage behavior)
	store, err := credentials.New()
	if err != nil {
		return fmt.Errorf("initialize credential store: %w", err)
	}

	cred := credentials.Token{
		Type:        credentials.TokenTypeOAuth,
		AccessToken: tok.Token,
		Username:    username,
	}

	usedInsecure, err := store.Save(cmd.Context(), provider, hostname, cred, secureOnly)
	if err != nil {
		return fmt.Errorf("failed to store GitLab token: %w", err)
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

// fetchGitLabUsername performs a best-effort lookup of the authenticated user's
// username via the GitLab REST API. Returns empty string on any failure so that
// login still succeeds (username is purely cosmetic for `auth status`).
func fetchGitLabUsername(baseURL, accessToken string) string {
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", baseURL+"/api/v4/user", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var u struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return ""
	}
	return u.Username
}
