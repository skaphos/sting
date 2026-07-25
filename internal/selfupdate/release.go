// SPDX-License-Identifier: MIT

package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"
)

const (
	repoOwner      = "skaphos"
	repoName       = "sting"
	defaultAPIBase = "https://api.github.com"
	releasesPage   = "https://github.com/skaphos/sting/releases"

	// checksumsName is the manifest every release publishes; bundleName is
	// the cosign bundle signed over it.
	checksumsName = "checksums.txt"
	bundleName    = "checksums.txt.sigstore.json"

	// maxAssetBytes bounds a download so a hostile or corrupt response
	// cannot exhaust memory. Release archives are single-digit megabytes.
	maxAssetBytes = 256 << 20
)

// ErrNotFound reports that a release, or an asset within it, does not exist.
var ErrNotFound = errors.New("not found")

// ErrRateLimited reports that the unauthenticated API quota is exhausted.
// sting deliberately sends no credential, so this is a real and expected
// failure mode rather than a reason to authenticate.
var ErrRateLimited = errors.New("rate limited")

// Release is the subset of a GitHub release this package needs.
type Release struct {
	Tag        string
	Draft      bool
	Prerelease bool
	Assets     map[string]string // asset name -> download URL
}

// AssetURL returns the download URL for name.
func (r *Release) AssetURL(name string) (string, error) {
	url, ok := r.Assets[name]
	if !ok {
		return "", fmt.Errorf("%w: release %s has no asset %q", ErrNotFound, r.Tag, name)
	}
	return url, nil
}

// AssetNames lists every asset in the release, for error messages that tell the
// user what does exist rather than only what does not.
func (r *Release) AssetNames() []string {
	names := make([]string, 0, len(r.Assets))
	for n := range r.Assets {
		names = append(names, n)
	}
	return names
}

// Client fetches releases. The HTTP client and API base are seams so the whole
// path is testable against httptest without touching the network.
type Client struct {
	HTTP    *http.Client
	APIBase string
}

// NewClient returns a Client pointed at the public GitHub API.
func NewClient() *Client {
	return &Client{
		HTTP:    &http.Client{Timeout: 60 * time.Second},
		APIBase: defaultAPIBase,
	}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *Client) apiBase() string {
	if c.APIBase != "" {
		return c.APIBase
	}
	return defaultAPIBase
}

// Latest resolves the most recent stable release. GitHub's "latest" endpoint
// excludes drafts and pre-releases, which is exactly the required behavior:
// neither may be selected as an update target unless named explicitly.
func (c *Client) Latest(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.apiBase(), repoOwner, repoName)
	return c.fetchRelease(ctx, url)
}

// ByTag resolves a specific release. Drafts and pre-releases are reachable
// here, and only here, because the user named one.
func (c *Client) ByTag(ctx context.Context, tag string) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", c.apiBase(), repoOwner, repoName, tag)
	return c.fetchRelease(ctx, url)
}

func (c *Client) fetchRelease(ctx context.Context, url string) (*Release, error) {
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}

	var payload struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decoding release: %w", err)
	}

	rel := &Release{
		Tag:        payload.TagName,
		Draft:      payload.Draft,
		Prerelease: payload.Prerelease,
		Assets:     make(map[string]string, len(payload.Assets)),
	}
	for _, a := range payload.Assets {
		rel.Assets[a.Name] = a.URL
	}
	return rel, nil
}

// Download fetches an asset by URL.
func (c *Client) Download(ctx context.Context, url string) ([]byte, error) {
	return c.get(ctx, url)
}

// get performs an unauthenticated request. It deliberately sends no
// Authorization header: release assets are public, and borrowing whatever
// credential happens to be in the environment is exactly what the constitution
// forbids. Nothing identifying is sent beyond the User-Agent the API requires.
func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sting-update")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("%w: %s", ErrNotFound, url)
	case rateLimited(resp):
		return nil, fmt.Errorf("%w: sting updates without a credential, so the "+
			"unauthenticated quota applies; retry after %s", ErrRateLimited, resetHint(resp))
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("fetching %s: unexpected status %s", url, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetBytes))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", url, err)
	}
	return body, nil
}

func rateLimited(resp *http.Response) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return resp.StatusCode == http.StatusForbidden &&
		resp.Header.Get("X-RateLimit-Remaining") == "0"
}

func resetHint(resp *http.Response) string {
	if v := resp.Header.Get("X-RateLimit-Reset"); v != "" {
		return "the window given by X-RateLimit-Reset (" + v + ")"
	}
	return "a short wait"
}

// AssetName returns the release archive for the running platform, matching the
// name_template in .goreleaser.yaml.
func AssetName(version string) string {
	return assetNameFor(version, runtime.GOOS, runtime.GOARCH)
}

func assetNameFor(version, goos, goarch string) string {
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("sting_%s_%s_%s.%s", strings.TrimPrefix(version, "v"), goos, goarch, ext)
}
