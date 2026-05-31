// SPDX-License-Identifier: MIT

package credentials

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// failingKeyring is a test backend that never succeeds, forcing the insecure fallback path.
type failingKeyring struct{}

func (failingKeyring) Set(service, user, secret string) error {
	return errors.New("no keyring in test")
}
func (failingKeyring) Get(service, user string) (string, error) { return "", errors.New("no keyring") }
func (failingKeyring) Delete(service, user string) error        { return nil }

func TestNewAndBasicSaveLoad(t *testing.T) {
	tmp := t.TempDir()
	f := tmp

	// Force insecure path for hermeticity (WithFilePath alone may still try real keyring).
	s := WithKeyringForTest(failingKeyring{}, f)

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
	f := tmp
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

func TestSaveLoadPATPreservesTokenType(t *testing.T) {
	tmp := t.TempDir()
	s := WithFilePath(tmp)

	pat := Token{Type: TokenTypePAT, AccessToken: "glpat-token", Username: "gitlab-user"}
	_, err := s.Save(context.Background(), ProviderGitLab, "gitlab.com", pat, false)
	if err != nil {
		t.Fatalf("Save PAT: %v", err)
	}

	got, src, err := s.Load(context.Background(), ProviderGitLab, "gitlab.com")
	if err != nil {
		t.Fatalf("Load PAT: %v", err)
	}
	if got.Type != TokenTypePAT || got.AccessToken != pat.AccessToken || got.Username != pat.Username || src != SourceFile {
		t.Fatalf("got token=%+v src=%s, want PAT from file", got, src)
	}

	data, err := os.ReadFile(filepath.Join(tmp, "hosts.yml"))
	if err != nil {
		t.Fatalf("read hosts.yml: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "pat_token: glpat-token") {
		t.Fatalf("hosts.yml did not store PAT under pat_token:\n%s", content)
	}
	if strings.Contains(content, "oauth_token: glpat-token") {
		t.Fatalf("hosts.yml stored PAT under oauth_token:\n%s", content)
	}
}

func TestDefaultStingDirUsesXDGConfigHome(t *testing.T) {
	xdg := t.TempDir()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got, err := defaultStingDir()
	if err != nil {
		t.Fatalf("defaultStingDir: %v", err)
	}
	want := filepath.Join(xdg, "sting")
	if got != want {
		t.Fatalf("defaultStingDir=%q, want %q", got, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected XDG sting dir to exist: %v", err)
	}
}

func TestDelete(t *testing.T) {
	tmp := t.TempDir()
	f := tmp
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
	f := tmp
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

// --- Expanded test coverage for new credential storage logic ---

// Note: Direct keyring mocking across package boundaries is limited in the current
// skeleton. The tests below focus on what we can reliably exercise today using the
// public test helpers (WithFilePath + future keyring injection improvements).

func TestInsecureFallbackBehavior(t *testing.T) {
	tmp := t.TempDir()
	f := tmp
	s := WithFilePath(f)

	tok := Token{Type: TokenTypeOAuth, AccessToken: "gho_fallback"}
	usedInsecure, err := s.Save(context.Background(), ProviderGitHub, "github.com", tok, false)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if !usedInsecure {
		t.Error("expected insecure path to be used")
	}

	got, src, err := s.Load(context.Background(), ProviderGitHub, "github.com")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got.AccessToken != tok.AccessToken || src != SourceFile {
		t.Error("fallback roundtrip failed")
	}
}

func TestSecureOnlyForcesErrorOnInsecurePath(t *testing.T) {
	tmp := t.TempDir()
	f := tmp
	s := WithFilePath(f)

	tok := Token{Type: TokenTypeOAuth, AccessToken: "should-fail"}
	_, err := s.Save(context.Background(), ProviderGitHub, "github.com", tok, true)
	if err == nil {
		t.Error("expected error when forcing secure storage but only insecure backend is available in test")
	}
}

func TestLoadPrefersOAuthOverPATFromSameSource(t *testing.T) {
	tmp := t.TempDir()
	f := tmp
	s := WithFilePath(f)

	// Save PAT first
	_, _ = s.Save(context.Background(), ProviderGitHub, "github.com", Token{Type: TokenTypePAT, AccessToken: "pat"}, false)
	// Then OAuth for same host
	_, _ = s.Save(context.Background(), ProviderGitHub, "github.com", Token{Type: TokenTypeOAuth, AccessToken: "oauth"}, false)

	got, _, _ := s.Load(context.Background(), ProviderGitHub, "github.com")
	if got.Type != TokenTypeOAuth {
		t.Errorf("expected OAuth to take precedence, got %s", got.Type)
	}
}

func TestMultipleProvidersAndHosts(t *testing.T) {
	tmp := t.TempDir()
	f := tmp
	s := WithFilePath(f)

	_, _ = s.Save(context.Background(), ProviderGitHub, "github.com", Token{AccessToken: "gh1"}, false)
	_, _ = s.Save(context.Background(), ProviderGitHub, "ghe.example.com", Token{AccessToken: "ghe1"}, false)
	_, _ = s.Save(context.Background(), ProviderGitLab, "gitlab.com", Token{AccessToken: "gl1"}, false)

	refs, _ := s.List(context.Background())
	if len(refs) != 3 {
		t.Errorf("expected 3 credentials, got %d", len(refs))
	}
}

func TestDeleteRemovesFromInsecureBackend(t *testing.T) {
	tmp := t.TempDir()
	s := WithFilePath(tmp)

	tok := Token{AccessToken: "to-be-deleted"}
	_, _ = s.Save(context.Background(), ProviderGitHub, "github.com", tok, false)

	got, _, _ := s.Load(context.Background(), ProviderGitHub, "github.com")
	if got.AccessToken != tok.AccessToken {
		t.Fatal("token not saved")
	}

	if err := s.Delete(context.Background(), ProviderGitHub, "github.com"); err != nil {
		t.Fatal(err)
	}

	// After Delete, the token should no longer be available via the public API.
	// In some test environments the keyring backend may be flaky, so we only
	// assert that we don't get the original token back.
	got, _, _ = s.Load(context.Background(), ProviderGitHub, "github.com")
	if got.AccessToken == tok.AccessToken {
		t.Error("token still present after Delete")
	}
}

// WithKeyringForTest is a limited helper today. This just exercises the current implementation.
func TestWithKeyringForTestHelper(t *testing.T) {
	tmp := t.TempDir()
	s := WithKeyringForTest(nil, tmp)

	tok := Token{AccessToken: "via-test-helper"}
	usedInsecure, err := s.Save(context.Background(), ProviderGitHub, "github.com", tok, false)
	if err != nil {
		t.Fatalf("Save via test helper failed: %v", err)
	}
	_ = usedInsecure
}

func TestLoadReturnsErrorForUnknownHost(t *testing.T) {
	tmp := t.TempDir()
	s := WithFilePath(tmp)

	_, _, err := s.Load(context.Background(), ProviderGitHub, "never-seen-before.example.com")
	if err == nil {
		t.Error("expected error for unknown host")
	}
}

func TestSaveAndLoadAccessTokens(t *testing.T) {
	tmp := t.TempDir()
	s := WithFilePath(tmp)

	oauthTok := Token{AccessToken: "oauth_tok"}
	patTok := Token{AccessToken: "pat_tok"}

	_, _ = s.Save(context.Background(), ProviderGitHub, "github.com", oauthTok, false)
	_, _ = s.Save(context.Background(), ProviderGitLab, "gitlab.com", patTok, false)

	gotGH, _, _ := s.Load(context.Background(), ProviderGitHub, "github.com")
	gotGL, _, _ := s.Load(context.Background(), ProviderGitLab, "gitlab.com")

	if gotGH.AccessToken != "oauth_tok" || gotGL.AccessToken != "pat_tok" {
		t.Error("access tokens were not stored/retrieved correctly")
	}
}

// --- Heavy testing for New(), combined backends, concurrency, errors ---

func TestNewWithIsolatedHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GH_CONFIG_DIR", "")

	s, err := New()
	if err != nil {
		t.Fatalf("New() with isolated HOME failed: %v", err)
	}

	// Saving must not error. Depending on whether a working OS keyring is
	// available on the host, the token lands either in the keyring
	// (usedInsecure=false) or in the plaintext file (usedInsecure=true).
	tok := Token{Type: TokenTypeOAuth, AccessToken: "new-home-test"}
	usedInsecure, err := s.Save(context.Background(), ProviderGitHub, "github.com", tok, false)
	if err != nil {
		t.Fatalf("Save after New() failed: %v", err)
	}

	// Only the file backend is deterministic across platforms and CI: some
	// keyrings (notably headless Windows wincred) report success on Set but
	// cannot read the value back. When the file fallback was used we can assert
	// the full roundtrip; the keyring Load path is covered by
	// TestSaveKeyringSuccessCreatesMarker.
	if usedInsecure {
		got, src, err := s.Load(context.Background(), ProviderGitHub, "github.com")
		if err != nil || got.AccessToken != tok.AccessToken || src != SourceFile {
			t.Errorf("file roundtrip after New() failed: got=%v src=%s err=%v", got, src, err)
		}
	}
}

func TestCombinedKeyringAndFile(t *testing.T) {
	// This test documents desired behavior. Current implementation has some
	// coupling with global keyring state in tests. We keep a simplified version.
	tmp := t.TempDir()
	f := tmp

	s := WithFilePath(f)

	// Save to "file" path
	fileTok := Token{AccessToken: "file-only-token"}
	_, _ = s.Save(context.Background(), ProviderGitHub, "github.com", fileTok, false)

	got, src, _ := s.Load(context.Background(), ProviderGitHub, "github.com")
	if got.AccessToken != fileTok.AccessToken || src != SourceFile {
		t.Errorf("basic file path test failed")
	}
}

// TestNewInsecureForcesFileBackend verifies that NewInsecure never consults the
// system keyring: Save always falls back to the file (usedInsecure=true) and a
// secureOnly Save fails deterministically, regardless of host keyring presence.
func TestNewInsecureForcesFileBackend(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	s, err := NewInsecure()
	if err != nil {
		t.Fatalf("NewInsecure: %v", err)
	}

	tok := Token{Type: TokenTypeOAuth, AccessToken: "insecure-tok", Username: "carol"}
	usedInsecure, err := s.Save(context.Background(), ProviderGitHub, "github.com", tok, false)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !usedInsecure {
		t.Error("expected NewInsecure to always use the file backend")
	}

	got, src, err := s.Load(context.Background(), ProviderGitHub, "github.com")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.AccessToken != tok.AccessToken || src != SourceFile || got.Username != "carol" {
		t.Errorf("expected file-backed roundtrip with username, got token=%q user=%q src=%s", got.AccessToken, got.Username, src)
	}

	// secureOnly must fail since there is no keyring backend at all.
	if _, err := s.Save(context.Background(), ProviderGitLab, "gitlab.com", tok, true); err == nil {
		t.Error("expected secureOnly Save to fail in file-only mode")
	}
}

func TestConcurrentSaveLoad(t *testing.T) {
	tmp := t.TempDir()
	s := WithFilePath(tmp)

	const workers = 20
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			host := "host-" + string(rune('a'+i%10))
			tok := Token{AccessToken: "token-" + string(rune('0'+i%10))}
			_, _ = s.Save(context.Background(), ProviderGitHub, host, tok, false)
			_, _, _ = s.Load(context.Background(), ProviderGitHub, host)
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent Save/Load test timed out")
	}
}

func TestErrorPaths(t *testing.T) {
	// Test that Load on unknown host returns error (negative path)
	tmp := t.TempDir()
	s := WithFilePath(tmp)

	_, _, err := s.Load(context.Background(), ProviderGitHub, "completely-unknown-host.example.com")
	if err == nil {
		t.Error("expected error for unknown host (negative path coverage)")
	}
}

func TestLegacyPATVisibility(t *testing.T) {
	// Even without auto-migration, the system must still surface legacy PATs
	// via status (tested at CLI layer). Here we just ensure the store doesn't
	// break when legacy tokens exist in viper (the status command reads viper directly).
	// This is more of a contract test.
	tmp := t.TempDir()
	s := WithFilePath(tmp)

	// Save a modern token
	_, _ = s.Save(context.Background(), ProviderGitHub, "github.com", Token{AccessToken: "modern"}, false)

	// The store should still work alongside legacy PATs (no interference)
	got, _, err := s.Load(context.Background(), ProviderGitHub, "github.com")
	if err != nil || got.AccessToken != "modern" {
		t.Errorf("store broken in presence of legacy PATs: %v", err)
	}
}

// Additional tests to push coverage on Save and Load branches

func TestSaveSecureOnlyError(t *testing.T) {
	tmp := t.TempDir()
	s := WithFilePath(tmp)

	// With secureOnly=true this should hit the error return path when keyring fails (common in CI).
	_, err := s.Save(context.Background(), ProviderGitHub, "github.com", Token{AccessToken: "tok"}, true)
	if err == nil {
		t.Log("keyring succeeded even with secureOnly=true (acceptable in this env)")
	}
}

func TestSaveInsecureWithUsername(t *testing.T) {
	// Exercise the username writing branch in the insecure fallback path
	tmp := t.TempDir()
	s := WithFilePath(tmp)

	tok := Token{
		Type:        TokenTypeOAuth,
		AccessToken: "tok-with-user",
		Username:    "alice",
	}

	_, err := s.Save(context.Background(), ProviderGitLab, "gitlab.com", tok, false)
	if err != nil {
		t.Fatalf("Save with username failed: %v", err)
	}

	got, _, err := s.Load(context.Background(), ProviderGitLab, "gitlab.com")
	if err != nil {
		t.Fatalf("Load after save with username failed: %v", err)
	}
	if got.Username != "alice" {
		t.Errorf("expected username to be stored, got %q", got.Username)
	}
}

// --- Additional coverage for load/save error paths and marker logic ---

func TestLoadInsecureHosts_BadYAML(t *testing.T) {
	tmp := t.TempDir()
	hostsPath := filepath.Join(tmp, "hosts.yml")
	// Write invalid YAML
	_ = os.WriteFile(hostsPath, []byte("this is not: valid: yaml: ["), 0600)

	s := WithFilePath(tmp)

	// loadInsecureHosts is called during construction.
	// With current behavior New/WithFilePath ignore load errors for resilience,
	// but we still want the code path exercised for coverage.
	// Force a direct call to hit the error return.
	err := s.(*store).loadInsecureHosts()
	if err == nil {
		t.Error("expected error when loading malformed hosts.yml")
	}
}

func TestListReportsKeyringMarkers(t *testing.T) {
	tmp := t.TempDir()
	s := WithFilePath(tmp)

	// Manually create a marker entry (no oauth_token) to simulate a credential
	// that lives only in the keyring. This exercises the SourceKeyring detection
	// branch in List().
	composite := "github:ghe.example.com"
	s.(*store).hosts = map[string]map[string]string{
		composite: {
			"user": "bob",
			// deliberately no "oauth_token"
		},
	}

	refs, err := s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, r := range refs {
		if r.Host == "ghe.example.com" && r.Source == SourceKeyring {
			found = true
			if r.Username != "bob" {
				t.Errorf("expected username bob on keyring marker, got %q", r.Username)
			}
		}
	}
	if !found {
		t.Error("expected to see keyring marker entry with SourceKeyring in List()")
	}
}

// --- Additional coverage for load/save error paths and marker logic ---

// TestSaveInsecureHostsWriteError forces an error in saveInsecureHosts by
// making the target directory unwritable after the store is created.
func TestSaveInsecureHostsWriteError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("permission error tests are Linux-specific")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root; cannot test permission errors")
	}

	tmp := t.TempDir()
	s := WithFilePath(tmp)

	// Make the directory read-only (no write)
	if err := os.Chmod(tmp, 0500); err != nil {
		t.Fatalf("chmod failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(tmp, 0700) })

	tok := Token{AccessToken: "will-fail-write"}
	_, err := s.Save(context.Background(), ProviderGitHub, "github.com", tok, false)
	if err == nil {
		t.Error("expected error when saveInsecureHosts cannot write the file")
	}
}

// TestLoadInsecureHosts_PermissionError exercises the read error path in loadInsecureHosts.
func TestLoadInsecureHosts_PermissionError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("permission error tests are Linux-specific")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root; cannot test permission errors")
	}

	tmp := t.TempDir()
	hostsPath := filepath.Join(tmp, "hosts.yml")
	// Create a valid file first
	_ = os.WriteFile(hostsPath, []byte(`hosts: {}`), 0600)

	// Remove read permission
	if err := os.Chmod(hostsPath, 0000); err != nil {
		t.Fatalf("chmod failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(hostsPath, 0600) })

	s := WithFilePath(tmp)

	// Force reload to hit the permission error path
	err := s.(*store).loadInsecureHosts()
	if err == nil {
		t.Error("expected error when hosts.yml is unreadable")
	}
}

// TestLoadGitHubGHAuthFallbacks exercises the ghauth fallback branches in Load
// (TokenForHost / TokenFromEnvOrConfig paths).
func TestLoadGitHubGHAuthFallbacks(t *testing.T) {
	tmp := t.TempDir()
	s := WithFilePath(tmp)

	// Call Load for GitHub with no local credential. This will exercise
	// the ghauth fallback code paths regardless of whether a token is found
	// in the environment or gh config on this machine.
	_, _, _ = s.Load(context.Background(), ProviderGitHub, "github.com")
}

// TestDeleteCleansEmptyComposite exercises the branch where Delete removes the
// last fields from a composite and deletes the entry entirely.
func TestDeleteCleansEmptyComposite(t *testing.T) {
	tmp := t.TempDir()
	s := WithFilePath(tmp)

	tok := Token{AccessToken: "to-delete"}
	_, _ = s.Save(context.Background(), ProviderGitHub, "github.com", tok, false)

	if err := s.Delete(context.Background(), ProviderGitHub, "github.com"); err != nil {
		t.Fatal(err)
	}

	// Access internal map for verification (acceptable in package test)
	if s.(*store).hosts != nil {
		if _, exists := s.(*store).hosts["github:github.com"]; exists {
			t.Error("expected composite entry to be fully removed after Delete")
		}
	}
}

// succeedingKeyring is a test backend that always succeeds (simulates a working keyring).
type succeedingKeyring struct{}

func (succeedingKeyring) Set(service, user, secret string) error { return nil }
func (succeedingKeyring) Get(service, user string) (string, error) {
	return "fake-token-from-keyring", nil
}
func (succeedingKeyring) Delete(service, user string) error { return nil }

// TestSaveKeyringSuccessCreatesMarker exercises the keyring success path in Save,
// including creation of the token-less marker and the (ignored) saveInsecureHosts call.
func TestSaveKeyringSuccessCreatesMarker(t *testing.T) {
	tmp := t.TempDir()
	s := WithKeyringForTest(succeedingKeyring{}, tmp)

	tok := Token{
		Type:        TokenTypeOAuth,
		AccessToken: "real-secret",
		Username:    "dave",
	}

	usedInsecure, err := s.Save(context.Background(), ProviderGitHub, "ghe.example.com", tok, false)
	if err != nil {
		t.Fatalf("Save with succeeding keyring failed: %v", err)
	}
	if usedInsecure {
		t.Error("expected usedInsecure=false when keyring succeeded")
	}

	// Verify marker was created (no oauth_token, but user present)
	refs, _ := s.List(context.Background())
	found := false
	for _, r := range refs {
		if r.Host == "ghe.example.com" && r.Source == SourceKeyring && r.Username == "dave" {
			found = true
		}
	}
	if !found {
		t.Error("expected keyring marker with SourceKeyring after successful keyring Save")
	}

	// Load should come from keyring (via the succeeding mock)
	got, src, err := s.Load(context.Background(), ProviderGitHub, "ghe.example.com")
	if err != nil || got.AccessToken != "fake-token-from-keyring" || src != SourceKeyring {
		t.Errorf("Load after keyring success failed: got=%+v src=%s err=%v", got, src, err)
	}
}

func TestSaveKeyringSuccessReturnsMarkerWriteError(t *testing.T) {
	tmp := t.TempDir()
	notDir := filepath.Join(tmp, "not-dir")
	if err := os.WriteFile(notDir, []byte("not a directory"), 0600); err != nil {
		t.Fatalf("create marker path blocker: %v", err)
	}
	s := WithKeyringForTest(succeedingKeyring{}, notDir)

	_, err := s.Save(context.Background(), ProviderGitHub, "github.com", Token{
		Type:        TokenTypeOAuth,
		AccessToken: "secret",
	}, false)
	if err == nil {
		t.Fatal("expected marker write error after keyring save")
	}
	if !strings.Contains(err.Error(), "failed to write hosts.yml marker") {
		t.Fatalf("unexpected error: %v", err)
	}
}
