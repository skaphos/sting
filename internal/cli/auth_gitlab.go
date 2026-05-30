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
	Long: `Authenticate with GitLab using OAuth (recommended).

Official Skaphos/Sting OAuth App credentials are included, so this works out of the box
for gitlab.com (no client secret required — the public app is registered as non-confidential).

Self-hosted GitLab:
  Use --hostname to target your instance.
  You will need to register your own OAuth Application and pass at least --client-id
  (or STING_GITLAB_CLIENT_ID). A client secret is only needed if you registered the
  application as confidential.

  Example:
    sting auth gitlab --hostname gitlab.example.com --client-id YOUR_CLIENT_ID

By default uses the device authorization flow (no browser required on the machine running sting).
Use --web to automatically open the verification page in your browser.

Examples:
  sting auth gitlab
  sting auth login gitlab
  sting auth gitlab --web
  sting auth gitlab --hostname gitlab.example.com --client-id YOUR_ID`,
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
	authGitLabCmd.Flags().StringVar(&authGitLabClientSecret, "client-secret", "", "OAuth application Client Secret (only needed for confidential apps)")
	authGitLabCmd.Flags().BoolVarP(&authGitLabClipboard, "clipboard", "c", false, "Copy the user code to the clipboard")
	authGitLabCmd.Flags().BoolVarP(&authGitLabWeb, "web", "w", false, "Open the verification URL in your browser automatically")
	authGitLabCmd.Flags().BoolVar(&authGitLabInsecure, "insecure-storage", false, "Save the token to the config file instead of the system keyring")
}

//nolint:errcheck // fmt.Fprint* calls are for human CLI output; stdout write failures are not actionable here.
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

	// Official Skaphos/Sting OAuth App for gitlab.com (non-confidential).
	// Only the Client ID is embedded. Client secrets are never defaulted and are
	// only used when explicitly provided (for users who registered confidential
	// apps on self-hosted instances).
	clientID := authGitLabClientID
	if clientID == "" {
		clientID = os.Getenv("STING_GITLAB_CLIENT_ID")
	}
	if clientID == "" {
		clientID = "c9766f569e9be5ee467fe3c50d5c8e44baec72e86132e4e1d7b761827bc448f0"
	}

	clientSecret := authGitLabClientSecret
	if clientSecret == "" {
		clientSecret = os.Getenv("STING_GITLAB_CLIENT_SECRET")
	}
	// clientSecret may remain empty — this is correct and expected for
	// non-confidential GitLab applications.

	// Self-hosted GitLab detection: if targeting anything other than gitlab.com
	// while still using the built-in public Client ID, show a clear error.
	isSelfHosted := hostname != "gitlab.com"
	usingPublicApp := authGitLabClientID == "" && os.Getenv("STING_GITLAB_CLIENT_ID") == ""

	if isSelfHosted && usingPublicApp {
		//lint:ignore ST1005 user-facing CLI error with proper punctuation and newlines
		//nolint:staticcheck // ST1005
		return fmt.Errorf(`Self-hosted GitLab detected (%s) — built-in Skaphos credentials only work against gitlab.com.

You need to register an OAuth Application on your instance and provide its Client ID:

  sting auth gitlab --hostname %s --client-id YOUR_CLIENT_ID

A --client-secret is only required if you registered the application as confidential.

See docs/oauth-app-registration.md for the exact settings (you must enable "Device authorization grant flow" and use scope read_api).`, hostname, hostname)
	}

	baseURL := "https://" + hostname
	deviceURL := baseURL + "/oauth/authorize_device"
	tokenURL := baseURL + "/oauth/token"

	fmt.Fprintf(cmd.OutOrStdout(), "Requesting device code for %s...\n", hostname)

	code, err := device.RequestCode(http.DefaultClient, deviceURL, clientID, []string{"read_api"})
	if err != nil {
		if err == device.ErrUnsupported {
			return fmt.Errorf("this GitLab instance does not support device flow; use --with-token instead")
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
	defer resp.Body.Close() //nolint:errcheck // defer cannot easily check close error; body close failure is non-fatal for our use case

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
