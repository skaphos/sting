// SPDX-License-Identifier: MIT
package commitclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/internal/credentials"
	"github.com/skaphos/sting/model"
)

const testSecret = "s3cr3t-token-value"

// fakeStore returns one canned Load outcome; the other Store methods are unused.
type fakeStore struct {
	tok credentials.Token
	src credentials.Source
	err error
}

func (f fakeStore) Save(context.Context, credentials.Provider, string, credentials.Token, bool) (bool, error) {
	return false, nil
}

func (f fakeStore) Load(context.Context, credentials.Provider, string) (credentials.Token, credentials.Source, error) {
	return f.tok, f.src, f.err
}

func (f fakeStore) Delete(context.Context, credentials.Provider, string) error { return nil }

func (f fakeStore) List(context.Context) ([]credentials.CredentialRef, error) { return nil, nil }

func useStore(t *testing.T, s credentials.Store, err error) {
	t.Helper()
	orig := openCredentialStore
	openCredentialStore = func() (credentials.Store, error) { return s, err }
	t.Cleanup(func() { openCredentialStore = orig })
}

func TestResolutionReportsEverySource(t *testing.T) {
	notFound := fmt.Errorf("%w for github/github.com", credentials.ErrNotFound)
	locked := errors.New("a stored credential for github/github.com exists but the keyring is unavailable: keyring is locked")

	tests := []struct {
		name      string
		provider  model.Provider
		dedicated string
		baseURL   string
		store     fakeStore
		openErr   error
		wantToken string
		want      string
	}{
		{name: "dedicated PAT wins over a stored login", provider: model.ProviderGitHub, dedicated: testSecret,
			store:     fakeStore{tok: credentials.Token{Type: credentials.TokenTypeOAuth, AccessToken: "stored"}, src: credentials.SourceKeyring},
			wantToken: testSecret, want: "sting: credential: github github.com: dedicated PAT"},
		{name: "keyring oauth", provider: model.ProviderGitHub,
			store:     fakeStore{tok: credentials.Token{Type: credentials.TokenTypeOAuth, AccessToken: testSecret}, src: credentials.SourceKeyring},
			wantToken: testSecret, want: "sting: credential: github github.com: stored oauth token (keyring)"},
		{name: "file oauth", provider: model.ProviderGitHub,
			store:     fakeStore{tok: credentials.Token{Type: credentials.TokenTypeOAuth, AccessToken: testSecret}, src: credentials.SourceFile},
			wantToken: testSecret, want: "sting: credential: github github.com: stored oauth token (file)"},
		{name: "stored gitlab pat on a self-managed host", provider: model.ProviderGitLab, baseURL: "https://gitlab.example.com/api/v4",
			store:     fakeStore{tok: credentials.Token{Type: credentials.TokenTypePAT, AccessToken: testSecret}, src: credentials.SourceFile},
			wantToken: testSecret, want: "sting: credential: gitlab gitlab.example.com: stored pat (file)"},
		{name: "ambient environment opt-in", provider: model.ProviderGitHub,
			store:     fakeStore{tok: credentials.Token{Type: credentials.TokenTypeOAuth, AccessToken: testSecret}, src: credentials.SourceEnv},
			wantToken: testSecret, want: "sting: credential: github github.com: ambient token from environment (opt-in)"},
		{name: "ambient gh config opt-in on GHES", provider: model.ProviderGitHub, baseURL: "https://ghe.example.com/api/v3",
			store:     fakeStore{tok: credentials.Token{Type: credentials.TokenTypeOAuth, AccessToken: testSecret}, src: credentials.SourceConfig},
			wantToken: testSecret, want: "sting: credential: github ghe.example.com: ambient token from gh config (opt-in)"},
		{name: "nothing stored", provider: model.ProviderGitHub,
			store: fakeStore{err: notFound},
			want:  "sting: credential: github github.com: anonymous"},
		{name: "locked keyring is a failed lookup, not absence", provider: model.ProviderGitHub,
			store: fakeStore{err: locked},
			want:  "sting: credential: github github.com: anonymous (stored-credential lookup failed: " + locked.Error() + ")"},
		{name: "unreadable credentials file never echoes its contents", provider: model.ProviderGitLab,
			openErr: errors.New("cannot use credentials file hosts.yml: yaml: line 2: found " + testSecret),
			want:    "sting: credential: gitlab gitlab.com: anonymous (stored-credential lookup failed: credentials file could not be read)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useStore(t, tt.store, tt.openErr)
			cfg := config.Default()
			var token string
			var r resolution
			if tt.provider == model.ProviderGitLab {
				cfg.GitLabToken, cfg.GitLabBaseURL = tt.dedicated, tt.baseURL
				token, r = resolveGitLabToken(cfg)
			} else {
				cfg.Token, cfg.BaseURL = tt.dedicated, tt.baseURL
				token, r = resolveGitHubToken(cfg)
			}

			if token != tt.wantToken {
				t.Fatalf("token = %q, want %q", token, tt.wantToken)
			}
			if got := r.String(); got != tt.want {
				t.Fatalf("report =\n  %q\nwant\n  %q", got, tt.want)
			}
			if strings.Contains(r.String(), testSecret) {
				t.Fatalf("report leaked the secret: %q", r.String())
			}
		})
	}
}

// TestConstructorsReportOnlyWhenAsked covers every constructor that resolves a
// credential: exactly one line with a writer, and silence with no option or a
// nil writer (the CLI passes nil when verbose is off).
func TestConstructorsReportOnlyWhenAsked(t *testing.T) {
	useStore(t, fakeStore{err: credentials.ErrNotFound}, nil)
	cfg := config.Default()
	cfg.Token = testSecret

	build := map[string]func(...Option) error{
		"New": func(opts ...Option) error {
			_, err := New(cfg, model.Query{Provider: model.ProviderGitHub}, opts...)
			return err
		},
		"NewGitLab": func(opts ...Option) error {
			_, err := New(cfg, model.Query{Provider: model.ProviderGitLab}, opts...)
			return err
		},
		"NewActivity": func(opts ...Option) error {
			_, err := NewActivity(cfg, model.ActivityQuery{Provider: model.ProviderGitHub}, opts...)
			return err
		},
		"NewPR": func(opts ...Option) error {
			_, err := NewPR(cfg, opts...)
			return err
		},
	}

	for name, fn := range build {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := fn(WithCredentialReport(&buf)); err != nil {
				t.Fatalf("build: %v", err)
			}
			if lines := strings.Count(buf.String(), "\n"); lines != 1 || !strings.HasPrefix(buf.String(), "sting: credential: ") {
				t.Fatalf("want exactly one report line, got %q", buf.String())
			}
			if strings.Contains(buf.String(), testSecret) {
				t.Fatalf("report leaked the secret: %q", buf.String())
			}

			if err := fn(); err != nil {
				t.Fatalf("build without option: %v", err)
			}
			if err := fn(WithCredentialReport(nil)); err != nil {
				t.Fatalf("build with nil writer: %v", err)
			}
		})
	}
}
