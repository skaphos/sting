package credentials

import (
	"context"
	"path/filepath"
	"testing"
)

func TestNewAndBasicSaveLoad(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "creds.json")

	s := WithFilePath(f)

	tok := Token{
		Type:        TokenTypeOAuth,
		AccessToken: "gho_fake123",
		Username:    "octocat",
	}

	usedInsecure, err := s.Save(context.Background(), ProviderGitHub, "github.com", tok, false)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if !usedInsecure {
		t.Error("expected insecure fallback to be used in this test setup")
	}

	got, src, err := s.Load(context.Background(), ProviderGitHub, "github.com")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got.AccessToken != tok.AccessToken {
		t.Errorf("got token %q, want %q", got.AccessToken, tok.AccessToken)
	}
	if src != SourceFile {
		t.Errorf("got source %s, want %s", src, SourceFile)
	}
}

func TestPrecedenceOAuthOverPAT(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "creds.json")
	s := WithFilePath(f)

	// Save a PAT first
	pat := Token{Type: TokenTypePAT, AccessToken: "ghp_pat"}
	_, _ = s.Save(context.Background(), ProviderGitHub, "github.com", pat, false)

	// Now save an OAuth token for same host
	oauth := Token{Type: TokenTypeOAuth, AccessToken: "gho_oauth"}
	_, _ = s.Save(context.Background(), ProviderGitHub, "github.com", oauth, false)

	got, _, err := s.Load(context.Background(), ProviderGitHub, "github.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "gho_oauth" {
		t.Errorf("expected OAuth token to take precedence, got %s", got.AccessToken)
	}
}

func TestDelete(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "creds.json")
	s := WithFilePath(f)

	tok := Token{Type: TokenTypePAT, AccessToken: "tok"}
	_, _ = s.Save(context.Background(), ProviderGitLab, "gitlab.example.com", tok, false)

	if err := s.Delete(context.Background(), ProviderGitLab, "gitlab.example.com"); err != nil {
		t.Fatal(err)
	}

	_, _, err := s.Load(context.Background(), ProviderGitLab, "gitlab.example.com")
	if err == nil {
		t.Error("expected error after Delete, got none")
	}
}

func TestList(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "creds.json")
	s := WithFilePath(f)

	_, _ = s.Save(context.Background(), ProviderGitHub, "github.com", Token{AccessToken: "a"}, false)
	_, _ = s.Save(context.Background(), ProviderGitLab, "gitlab.com", Token{AccessToken: "b"}, false)

	refs, err := s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Errorf("expected 2 refs, got %d", len(refs))
	}
}
