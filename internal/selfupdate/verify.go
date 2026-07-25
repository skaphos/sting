// SPDX-License-Identifier: MIT

package selfupdate

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

// The pinned signer. Verification without these is decorative: a bundle signed
// by *any* Sigstore identity verifies successfully, so the identity pin is the
// entire control. sting's releases are signed keyless by its own release
// workflow, using the workflow's ambient GitHub Actions OIDC identity.
const (
	// ExpectedIssuer is the OIDC provider that issued the signing
	// certificate.
	ExpectedIssuer = "https://token.actions.githubusercontent.com"

	// ExpectedSANPattern matches the release workflow's certificate SAN.
	// Anchored at both ends so no other repository or workflow can match.
	ExpectedSANPattern = `^https://github\.com/skaphos/sting/\.github/workflows/release\.yml@refs/tags/[^/]+$`
)

// ErrVerification reports that the release could not be proven authentic. It
// is deliberately distinct from a transport error: a network failure means
// "try later", this means "do not install this".
var ErrVerification = errors.New("verification failed")

// expectedSAN is compiled once; a malformed pattern is a programming error, so
// it panics at init rather than degrading to a check that matches everything.
var expectedSAN = regexp.MustCompile(ExpectedSANPattern)

// trustedMaterialFunc supplies Sigstore's trusted root. It is a seam so tests
// can supply fixture material instead of fetching it.
type trustedMaterialFunc func() (root.TrustedMaterial, error)

func fetchTrustedRoot() (root.TrustedMaterial, error) {
	tr, err := root.FetchTrustedRoot()
	if err != nil {
		return nil, fmt.Errorf("obtaining Sigstore trusted root: %w", err)
	}
	return tr, nil
}

// Verifier checks that a release was published by sting's own release workflow
// and that the artifact matches what was signed.
//
// There is no option to skip any of this, by flag, environment variable, or
// config key, and none may be added.
type Verifier struct {
	trustedMaterial trustedMaterialFunc

	// allowMissingSCT drops the signed-certificate-timestamp requirement.
	// It exists only for tests that mint signatures with a virtual
	// certificate authority, which cannot produce SCTs; Fulcio embeds one
	// in every certificate it issues, so real releases always carry them.
	//
	// The field is unexported and reachable from no flag, environment
	// variable, or config key, and its zero value is the strict behavior --
	// a Verifier that is not deliberately relaxed is always strict.
	allowMissingSCT bool
}

// NewVerifier returns a Verifier using Sigstore's public trusted root.
func NewVerifier() *Verifier {
	return &Verifier{trustedMaterial: fetchTrustedRoot}
}

// verifierOptions returns the verification requirements. Every release is
// signed by Fulcio, which embeds a signed certificate timestamp, so requiring
// one is the correct production posture.
func (v *Verifier) verifierOptions() []verify.VerifierOption {
	opts := []verify.VerifierOption{
		verify.WithTransparencyLog(1),
		verify.WithObserverTimestamps(1),
	}
	if !v.allowMissingSCT {
		opts = append(opts, verify.WithSignedCertificateTimestamps(1))
	}
	return opts
}

// VerifyRelease runs the full chain in order, each step gating the next:
//
//  1. verify the cosign bundle over the checksum manifest,
//  2. assert the signing certificate is sting's release workflow,
//  3. read the artifact's digest from the now-trusted manifest,
//  4. hash the downloaded artifact and compare.
//
// Steps 1 and 2 happen together: the identity is part of the verification
// policy rather than a check applied afterwards, so there is no window in
// which a signature is accepted before its signer is known.
func (v *Verifier) VerifyRelease(checksums, bundleJSON, artifact []byte, assetName string) error {
	if err := v.verifyChecksums(checksums, bundleJSON); err != nil {
		return err
	}

	digests, err := ParseChecksums(checksums)
	if err != nil {
		return err
	}

	return VerifyArtifactDigest(assetName, artifact, digests)
}

// verifyChecksums performs steps 1 and 2, parsing the bundle before handing it
// to verifyEntity.
func (v *Verifier) verifyChecksums(checksums, bundleJSON []byte) error {
	var b bundle.Bundle
	if err := b.UnmarshalJSON(bundleJSON); err != nil {
		return fmt.Errorf("%w: parsing signature bundle: %w", ErrVerification, err)
	}
	return v.verifyEntity(&b, checksums)
}

// verifyEntity applies the signature and identity policy to an already-parsed
// signed entity. It is separated from bundle parsing so the policy itself can
// be exercised against entities minted at test time, rather than only against
// serialized fixtures.
func (v *Verifier) verifyEntity(entity verify.SignedEntity, checksums []byte) error {
	material, err := v.trustedMaterial()
	if err != nil {
		// Verification needs the trusted root. When it cannot be
		// obtained we refuse rather than falling back to checksums
		// alone -- an unverified checksum proves only that a download
		// matched whatever the same source served.
		return fmt.Errorf("%w: %w", ErrVerification, err)
	}

	sev, err := verify.NewVerifier(material, v.verifierOptions()...)
	if err != nil {
		return fmt.Errorf("%w: building verifier: %w", ErrVerification, err)
	}

	identity, err := certificateIdentity()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrVerification, err)
	}

	digest := sha256.Sum256(checksums)
	policy := verify.NewPolicy(
		verify.WithArtifactDigest("sha256", digest[:]),
		verify.WithCertificateIdentity(identity),
	)

	if _, err := sev.Verify(entity, policy); err != nil {
		return fmt.Errorf("%w: the checksum manifest is not signed by sting's "+
			"release workflow (%w)", ErrVerification, err)
	}
	return nil
}

// certificateIdentity builds the pinned identity policy. It always constrains
// both the issuer and the subject; there is no code path that produces an
// unconstrained policy.
func certificateIdentity() (verify.CertificateIdentity, error) {
	return verify.NewShortCertificateIdentity(ExpectedIssuer, "", "", ExpectedSANPattern)
}

// MatchesExpectedSigner reports whether a certificate's subject and issuer are
// sting's release workflow. Exported for the negative test that proves a valid
// signature from a different identity is rejected.
func MatchesExpectedSigner(san, issuer string) bool {
	return issuer == ExpectedIssuer && expectedSAN.MatchString(san)
}

// ParseChecksums reads a GoReleaser checksum manifest: "<hex>  <filename>"
// per line.
func ParseChecksums(manifest []byte) (map[string]string, error) {
	digests := make(map[string]string)

	scanner := bufio.NewScanner(bytes.NewReader(manifest))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("%w: malformed checksum line %q", ErrVerification, line)
		}
		digests[fields[1]] = strings.ToLower(fields[0])
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: reading checksum manifest: %w", ErrVerification, err)
	}
	if len(digests) == 0 {
		return nil, fmt.Errorf("%w: checksum manifest is empty", ErrVerification)
	}
	return digests, nil
}

// VerifyArtifactDigest performs steps 3 and 4: look up the expected digest in
// the verified manifest and compare it to what was actually downloaded.
func VerifyArtifactDigest(assetName string, artifact []byte, digests map[string]string) error {
	want, ok := digests[assetName]
	if !ok {
		return fmt.Errorf("%w: %q is not listed in the signed checksum manifest",
			ErrVerification, assetName)
	}

	sum := sha256.Sum256(artifact)
	got := hex.EncodeToString(sum[:])
	if got != want {
		return fmt.Errorf("%w: checksum mismatch for %s (expected %s, got %s)",
			ErrVerification, assetName, want, got)
	}
	return nil
}
