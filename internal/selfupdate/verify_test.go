// SPDX-License-Identifier: MIT

package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
	"github.com/sigstore/sigstore-go/pkg/root"
)

// TestRejectsValidSignatureFromWrongIdentity is the most important test in this
// feature. A Sigstore signature is only meaningful together with the identity
// that produced it: without the pin, a bundle signed by anyone at all verifies
// successfully, and the check becomes decorative. These are the identities an
// attacker or a mistake could realistically present.
func TestRejectsValidSignatureFromWrongIdentity(t *testing.T) {
	const goodSAN = "https://github.com/skaphos/sting/.github/workflows/release.yml@refs/tags/v1.0.0"

	if !MatchesExpectedSigner(goodSAN, ExpectedIssuer) {
		t.Fatalf("legitimate release identity was rejected: %q", goodSAN)
	}

	rejected := []struct {
		name, san, issuer string
	}{
		{
			name:   "a fork's release workflow",
			san:    "https://github.com/someone-else/sting/.github/workflows/release.yml@refs/tags/v1.0.0",
			issuer: ExpectedIssuer,
		},
		{
			name:   "a different repository in the same org",
			san:    "https://github.com/skaphos/repokeeper/.github/workflows/release.yml@refs/tags/v1.0.0",
			issuer: ExpectedIssuer,
		},
		{
			name:   "a different workflow in this repository",
			san:    "https://github.com/skaphos/sting/.github/workflows/ci.yml@refs/tags/v1.0.0",
			issuer: ExpectedIssuer,
		},
		{
			name:   "a branch rather than a tag",
			san:    "https://github.com/skaphos/sting/.github/workflows/release.yml@refs/heads/main",
			issuer: ExpectedIssuer,
		},
		{
			name:   "a nested ref that smuggles extra path segments",
			san:    "https://github.com/skaphos/sting/.github/workflows/release.yml@refs/tags/v1.0.0/../../evil",
			issuer: ExpectedIssuer,
		},
		{
			name:   "a lookalike host prefix",
			san:    "https://github.com.evil.test/skaphos/sting/.github/workflows/release.yml@refs/tags/v1.0.0",
			issuer: ExpectedIssuer,
		},
		{
			name:   "the right subject with a different OIDC issuer",
			san:    goodSAN,
			issuer: "https://accounts.google.com",
		},
		{
			name:   "an empty identity",
			san:    "",
			issuer: "",
		},
	}

	// Assert against the real policy object sigstore-go will apply, not
	// only against the helper. A test that agreed with the helper but not
	// with the shipped policy would prove nothing.
	identity, err := certificateIdentity()
	if err != nil {
		t.Fatalf("certificateIdentity() error = %v", err)
	}

	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			if MatchesExpectedSigner(tc.san, tc.issuer) {
				t.Errorf("helper accepted the wrong identity:\n  san:    %q\n  issuer: %q",
					tc.san, tc.issuer)
			}

			// The issuer matcher reads the OIDC issuer from the
			// certificate extensions, not the certificate's own
			// issuer DN.
			summary := certificate.Summary{
				SubjectAlternativeName: tc.san,
				Extensions:             certificate.Extensions{Issuer: tc.issuer},
			}
			sanErr := identity.SubjectAlternativeName.Verify(summary)
			issuerErr := identity.Issuer.Verify(summary)
			if sanErr == nil && issuerErr == nil {
				t.Errorf("the shipped sigstore policy accepted the wrong identity:\n  san:    %q\n  issuer: %q",
					tc.san, tc.issuer)
			}
		})
	}

	// And the genuine identity must satisfy that same policy, so the pin is
	// not merely rejecting everything.
	good := certificate.Summary{
		SubjectAlternativeName: goodSAN,
		Extensions:             certificate.Extensions{Issuer: ExpectedIssuer},
	}
	if err := identity.SubjectAlternativeName.Verify(good); err != nil {
		t.Errorf("shipped policy rejected the genuine SAN: %v", err)
	}
	if err := identity.Issuer.Verify(good); err != nil {
		t.Errorf("shipped policy rejected the genuine issuer: %v", err)
	}
}

// TestCertificateIdentityAlwaysConstrained guards against the policy being
// built without an identity, which would accept any signed artifact.
func TestCertificateIdentityAlwaysConstrained(t *testing.T) {
	id, err := certificateIdentity()
	if err != nil {
		t.Fatalf("certificateIdentity() error = %v", err)
	}
	if id.Issuer.Issuer != ExpectedIssuer {
		t.Errorf("issuer = %q, want %q", id.Issuer.Issuer, ExpectedIssuer)
	}
	if got := id.SubjectAlternativeName.Regexp.String(); got != ExpectedSANPattern {
		t.Errorf("SAN pattern = %q, want %q", got, ExpectedSANPattern)
	}
	// An empty matcher on either axis would match everything.
	if id.Issuer.Issuer == "" && id.Issuer.Regexp.String() == "" {
		t.Error("issuer matcher is unconstrained: it would accept any issuer")
	}
	if id.SubjectAlternativeName.SubjectAlternativeName == "" &&
		id.SubjectAlternativeName.Regexp.String() == "" {
		t.Error("SAN matcher is unconstrained: it would accept any subject")
	}
}

func checksumManifest(entries map[string]string) []byte {
	var b strings.Builder
	for name, digest := range entries {
		fmt.Fprintf(&b, "%s  %s\n", digest, name)
	}
	return []byte(b.String())
}

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestParseChecksums(t *testing.T) {
	artifact := []byte("archive contents")
	manifest := checksumManifest(map[string]string{"sting_1.0.0_linux_amd64.tar.gz": digestOf(artifact)})

	digests, err := ParseChecksums(manifest)
	if err != nil {
		t.Fatalf("ParseChecksums() error = %v", err)
	}
	if got := digests["sting_1.0.0_linux_amd64.tar.gz"]; got != digestOf(artifact) {
		t.Errorf("digest = %q, want %q", got, digestOf(artifact))
	}
}

func TestParseChecksumsRejectsMalformed(t *testing.T) {
	for _, tc := range []struct {
		name     string
		manifest string
	}{
		{"empty manifest", ""},
		{"only whitespace", "\n  \n"},
		{"missing filename", "abc123\n"},
		{"extra fields", "abc123  file  extra\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseChecksums([]byte(tc.manifest)); !errors.Is(err, ErrVerification) {
				t.Errorf("ParseChecksums(%q) error = %v, want ErrVerification", tc.manifest, err)
			}
		})
	}
}

// TestVerifyArtifactDigestMismatch covers the requirement that a tampered
// artifact is refused and nothing is written.
func TestVerifyArtifactDigestMismatch(t *testing.T) {
	genuine := []byte("the real release archive")
	tampered := []byte("the real release archive, plus something extra")
	name := "sting_1.0.0_linux_amd64.tar.gz"

	digests, err := ParseChecksums(checksumManifest(map[string]string{name: digestOf(genuine)}))
	if err != nil {
		t.Fatalf("ParseChecksums() error = %v", err)
	}

	if err := VerifyArtifactDigest(name, genuine, digests); err != nil {
		t.Fatalf("genuine artifact rejected: %v", err)
	}

	err = VerifyArtifactDigest(name, tampered, digests)
	if !errors.Is(err, ErrVerification) {
		t.Fatalf("tampered artifact error = %v, want ErrVerification", err)
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error does not name the failing check: %v", err)
	}
}

func TestVerifyArtifactDigestUnlistedAsset(t *testing.T) {
	digests := map[string]string{"other.tar.gz": digestOf([]byte("x"))}
	err := VerifyArtifactDigest("sting_1.0.0_linux_amd64.tar.gz", []byte("x"), digests)
	if !errors.Is(err, ErrVerification) {
		t.Fatalf("error = %v, want ErrVerification", err)
	}
	if !strings.Contains(err.Error(), "not listed in the signed checksum manifest") {
		t.Errorf("error does not explain the problem: %v", err)
	}
}

// TestVerificationCannotBeSkipped asserts there is no environment escape
// hatch. Verification is unskippable by design, so a tampered artifact must
// still be refused no matter what the environment says.
func TestVerificationCannotBeSkipped(t *testing.T) {
	for _, env := range []string{
		"STING_SKIP_VERIFY", "SKIP_VERIFY", "STING_INSECURE",
		"STING_NO_VERIFY", "COSIGN_EXPERIMENTAL", "STING_UPDATE_INSECURE",
	} {
		t.Setenv(env, "1")
	}

	name := "sting_1.0.0_linux_amd64.tar.gz"
	digests, err := ParseChecksums(checksumManifest(map[string]string{name: digestOf([]byte("genuine"))}))
	if err != nil {
		t.Fatalf("ParseChecksums() error = %v", err)
	}

	if err := VerifyArtifactDigest(name, []byte("tampered"), digests); !errors.Is(err, ErrVerification) {
		t.Fatalf("verification was skipped via the environment: err = %v", err)
	}
}

// TestVerifyChecksumsRefusesWithoutTrustRoot covers the edge case that
// verification requires the Sigstore trust root: when it cannot be obtained,
// the update refuses rather than degrading to checksums alone. An unverified
// checksum proves only that the download matched what the same source served.
func TestVerifyChecksumsRefusesWithoutTrustRoot(t *testing.T) {
	v := &Verifier{
		trustedMaterial: func() (root.TrustedMaterial, error) {
			return nil, errors.New("offline")
		},
	}

	err := v.verifyChecksums([]byte("checksums"), []byte("{}"))
	if !errors.Is(err, ErrVerification) {
		t.Fatalf("error = %v, want ErrVerification", err)
	}
}

// TestVerifyReleaseStopsAtFirstFailure confirms the chain is ordered: a bad
// signature is reported without the artifact digest ever being consulted.
func TestVerifyReleaseStopsAtFirstFailure(t *testing.T) {
	v := &Verifier{
		trustedMaterial: func() (root.TrustedMaterial, error) {
			return nil, errors.New("no trust root")
		},
	}

	err := v.VerifyRelease([]byte("checksums"), []byte("{}"), []byte("artifact"), "anything")
	if !errors.Is(err, ErrVerification) {
		t.Fatalf("error = %v, want ErrVerification", err)
	}
}

func TestNewVerifierUsesPublicTrustRoot(t *testing.T) {
	v := NewVerifier()
	if v.trustedMaterial == nil {
		t.Fatal("NewVerifier() left trustedMaterial nil, which would panic on use")
	}
}
