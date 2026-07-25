// SPDX-License-Identifier: MIT

package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// releaseJSON renders a GitHub release payload.
func releaseJSON(tag string, assets map[string]string, draft, prerelease bool) string {
	type asset struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	}
	payload := struct {
		TagName    string  `json:"tag_name"`
		Draft      bool    `json:"draft"`
		Prerelease bool    `json:"prerelease"`
		Assets     []asset `json:"assets"`
	}{TagName: tag, Draft: draft, Prerelease: prerelease}

	for name, url := range assets {
		payload.Assets = append(payload.Assets, asset{Name: name, URL: url})
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

// newTestClient wires a Client to an httptest server. No test in this package
// touches the network.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Client{HTTP: srv.Client(), APIBase: srv.URL}
}

func TestLatestRelease(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/releases/latest") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(releaseJSON("v1.2.0", map[string]string{
			"sting_1.2.0_linux_amd64.tar.gz": "https://example.test/archive",
			checksumsName:                    "https://example.test/checksums",
		}, false, false)))
	})

	rel, err := c.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if rel.Tag != "v1.2.0" {
		t.Errorf("Tag = %q, want v1.2.0", rel.Tag)
	}
	if url, err := rel.AssetURL(checksumsName); err != nil || url != "https://example.test/checksums" {
		t.Errorf("AssetURL(%s) = %q, %v", checksumsName, url, err)
	}
}

func TestReleaseByTag(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/releases/tags/v0.9.0") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(releaseJSON("v0.9.0", nil, false, false)))
	})

	rel, err := c.ByTag(context.Background(), "v0.9.0")
	if err != nil {
		t.Fatalf("ByTag() error = %v", err)
	}
	if rel.Tag != "v0.9.0" {
		t.Errorf("Tag = %q, want v0.9.0", rel.Tag)
	}
}

// TestPrereleaseReachableOnlyByName documents the division of responsibility:
// GitHub's "latest" endpoint excludes drafts and pre-releases, and naming a tag
// explicitly is the only way to reach one.
func TestPrereleaseReachableOnlyByName(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			// The endpoint never returns the pre-release.
			_, _ = w.Write([]byte(releaseJSON("v1.0.0", nil, false, false)))
			return
		}
		_, _ = w.Write([]byte(releaseJSON("v2.0.0-rc1", nil, false, true)))
	})

	latest, err := c.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if latest.Prerelease {
		t.Error("Latest() returned a pre-release")
	}

	named, err := c.ByTag(context.Background(), "v2.0.0-rc1")
	if err != nil {
		t.Fatalf("ByTag() error = %v", err)
	}
	if !named.Prerelease {
		t.Error("explicitly named pre-release was not reported as one")
	}
}

func TestReleaseNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if _, err := c.ByTag(context.Background(), "v9.9.9"); !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

// TestRateLimited covers the consequence of sending no credential: the
// unauthenticated quota applies, and exhausting it must produce a clear message
// rather than an obscure 403.
func TestRateLimited(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		header map[string]string
	}{
		{"403 with exhausted quota", http.StatusForbidden, map[string]string{
			"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": "1800000000",
		}},
		{"429 too many requests", http.StatusTooManyRequests, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tc.header {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tc.status)
			})

			_, err := c.Latest(context.Background())
			if !errors.Is(err, ErrRateLimited) {
				t.Fatalf("error = %v, want ErrRateLimited", err)
			}
			if !strings.Contains(err.Error(), "without a credential") {
				t.Errorf("error does not explain why the quota is low: %v", err)
			}
		})
	}
}

// TestNoCredentialIsEverSent is a constitutional requirement: sting
// authenticates with its own PATs and must never borrow an ambient provider
// token. Release assets are public, so the update path sends nothing at all.
func TestNoCredentialIsEverSent(t *testing.T) {
	// Populate every credential the process might be able to reach.
	t.Setenv("GITHUB_TOKEN", "ghp_ambient_should_not_be_used")
	t.Setenv("GH_TOKEN", "ghp_ambient_should_not_be_used")
	t.Setenv("STING_TOKEN", "sting_own_token_also_not_used")

	var sawAuth, sawCookie string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		sawCookie = r.Header.Get("Cookie")
		_, _ = w.Write([]byte(releaseJSON("v1.0.0", nil, false, false)))
	})

	if _, err := c.Latest(context.Background()); err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if sawAuth != "" {
		t.Errorf("update path sent an Authorization header: %q", sawAuth)
	}
	if sawCookie != "" {
		t.Errorf("update path sent a Cookie header: %q", sawCookie)
	}
}

func TestUnexpectedStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if _, err := c.Latest(context.Background()); err == nil {
		t.Fatal("expected an error for a 500 response")
	}
}

func TestMalformedReleasePayload(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{not json"))
	})
	if _, err := c.Latest(context.Background()); err == nil {
		t.Fatal("expected an error for a malformed payload")
	}
}

func TestDownload(t *testing.T) {
	want := []byte("archive bytes")
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(want)
	})

	got, err := c.Download(context.Background(), c.APIBase+"/asset")
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("Download() = %q, want %q", got, want)
	}
}

func TestAssetURLMissing(t *testing.T) {
	rel := &Release{Tag: "v1.0.0", Assets: map[string]string{"other": "u"}}
	if _, err := rel.AssetURL("sting_1.0.0_linux_amd64.tar.gz"); !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
	if names := rel.AssetNames(); len(names) != 1 || names[0] != "other" {
		t.Errorf("AssetNames() = %v", names)
	}
}

// TestAssetNameMatchesGoreleaserTemplate pins the naming to the template in
// .goreleaser.yaml. If that template changes, updates break silently, so this
// is the guard.
func TestAssetNameMatchesGoreleaserTemplate(t *testing.T) {
	for _, tt := range []struct {
		version, goos, goarch, want string
	}{
		{"v1.2.0", "linux", "amd64", "sting_1.2.0_linux_amd64.tar.gz"},
		{"1.2.0", "linux", "arm64", "sting_1.2.0_linux_arm64.tar.gz"},
		{"v1.2.0", "darwin", "arm64", "sting_1.2.0_darwin_arm64.tar.gz"},
		{"v1.2.0", "windows", "amd64", "sting_1.2.0_windows_amd64.zip"},
	} {
		if got := assetNameFor(tt.version, tt.goos, tt.goarch); got != tt.want {
			t.Errorf("assetNameFor(%q, %q, %q) = %q, want %q",
				tt.version, tt.goos, tt.goarch, got, tt.want)
		}
	}

	if got := AssetName("v1.2.0"); got == "" {
		t.Error("AssetName() returned empty for the running platform")
	}
}

func TestNewClientDefaults(t *testing.T) {
	c := NewClient()
	if c.apiBase() != defaultAPIBase {
		t.Errorf("apiBase() = %q, want %q", c.apiBase(), defaultAPIBase)
	}
	if c.httpClient() == nil {
		t.Error("httpClient() returned nil")
	}

	bare := &Client{}
	if bare.httpClient() != http.DefaultClient {
		t.Error("a zero Client did not fall back to http.DefaultClient")
	}
	if bare.apiBase() != defaultAPIBase {
		t.Error("a zero Client did not fall back to the default API base")
	}
}
