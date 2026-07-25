// SPDX-License-Identifier: MIT

package selfupdate

import (
	"errors"
	"strings"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/testing/ca"
)

// These tests exercise the real sigstore verification path against signatures
// minted at test time, rather than asserting on the matcher in isolation. A
// test that agreed with the identity regex but not with what sigstore actually
// enforces would prove nothing about what ships.

const genuineSAN = "https://github.com/skaphos/sting/.github/workflows/release.yml@refs/tags/v1.0.0"

// TestVerifyEntityAcceptsGenuineSigner: a signature produced by sting's own
// release workflow identity passes the shipped policy end to end.
func TestVerifyEntityAcceptsGenuineSigner(t *testing.T) {
	sigstore, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatalf("creating virtual sigstore: %v", err)
	}

	checksums := []byte("abc123  sting_1.0.0_linux_amd64.tar.gz\n")
	entity, err := sigstore.Sign(genuineSAN, ExpectedIssuer, checksums)
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	v := &Verifier{trustedMaterial: trustedMaterialOf(sigstore), allowMissingSCT: true}
	if err := v.verifyEntity(entity, checksums); err != nil {
		t.Fatalf("genuine signature was rejected: %v", err)
	}
}

// TestVerifyEntityRejectsWrongSigner is the requirement in its strongest form:
// a cryptographically valid, transparency-logged signature, from a real
// certificate authority, is still refused because it was not sting's release
// workflow that produced it.
func TestVerifyEntityRejectsWrongSigner(t *testing.T) {
	sigstore, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatalf("creating virtual sigstore: %v", err)
	}

	checksums := []byte("abc123  sting_1.0.0_linux_amd64.tar.gz\n")

	for _, tc := range []struct{ name, san, issuer string }{
		{
			name:   "a fork's release workflow",
			san:    "https://github.com/someone-else/sting/.github/workflows/release.yml@refs/tags/v1.0.0",
			issuer: ExpectedIssuer,
		},
		{
			name:   "another workflow in this repository",
			san:    "https://github.com/skaphos/sting/.github/workflows/ci.yml@refs/tags/v1.0.0",
			issuer: ExpectedIssuer,
		},
		{
			name:   "the right subject from a different issuer",
			san:    genuineSAN,
			issuer: "https://accounts.google.com",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entity, err := sigstore.Sign(tc.san, tc.issuer, checksums)
			if err != nil {
				t.Fatalf("signing: %v", err)
			}

			v := &Verifier{trustedMaterial: trustedMaterialOf(sigstore), allowMissingSCT: true}
			err = v.verifyEntity(entity, checksums)
			if err == nil {
				t.Fatalf("a valid signature from %q was accepted", tc.san)
			}
			if !errors.Is(err, ErrVerification) {
				t.Errorf("error = %v, want ErrVerification", err)
			}
		})
	}
}

// TestVerifyEntityRejectsMismatchedArtifact: the signature is genuine, but it
// covers different bytes than the ones downloaded.
func TestVerifyEntityRejectsMismatchedArtifact(t *testing.T) {
	sigstore, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatalf("creating virtual sigstore: %v", err)
	}

	signed := []byte("abc123  sting_1.0.0_linux_amd64.tar.gz\n")
	entity, err := sigstore.Sign(genuineSAN, ExpectedIssuer, signed)
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	tampered := []byte("deadbeef  sting_1.0.0_linux_amd64.tar.gz\n")
	v := &Verifier{trustedMaterial: trustedMaterialOf(sigstore), allowMissingSCT: true}
	if err := v.verifyEntity(entity, tampered); err == nil {
		t.Fatal("a signature over different content was accepted")
	}
}

// TestVerifyEntityRejectsForeignTrustRoot: a signature from a completely
// separate Sigstore instance must not verify against ours, even with the right
// identity strings.
func TestVerifyEntityRejectsForeignTrustRoot(t *testing.T) {
	ours, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatalf("creating virtual sigstore: %v", err)
	}
	theirs, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatalf("creating second virtual sigstore: %v", err)
	}

	checksums := []byte("abc123  sting_1.0.0_linux_amd64.tar.gz\n")
	entity, err := theirs.Sign(genuineSAN, ExpectedIssuer, checksums)
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	v := &Verifier{trustedMaterial: trustedMaterialOf(ours), allowMissingSCT: true}
	if err := v.verifyEntity(entity, checksums); err == nil {
		t.Fatal("a signature rooted in a different trust root was accepted")
	}
}

// TestVerifyChecksumsRejectsMalformedBundle covers the JSON parsing boundary.
func TestVerifyChecksumsRejectsMalformedBundle(t *testing.T) {
	sigstore, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatalf("creating virtual sigstore: %v", err)
	}
	v := &Verifier{trustedMaterial: trustedMaterialOf(sigstore), allowMissingSCT: true}

	for _, tc := range []struct{ name, payload string }{
		{"not json", "not a bundle at all"},
		{"empty object", "{}"},
		{"truncated", `{"mediaType":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := v.verifyChecksums([]byte("checksums"), []byte(tc.payload))
			if !errors.Is(err, ErrVerification) {
				t.Errorf("error = %v, want ErrVerification", err)
			}
			if !strings.Contains(err.Error(), "verification failed") {
				t.Errorf("error is not labelled as a verification failure: %v", err)
			}
		})
	}
}

// trustedMaterialOf adapts a virtual sigstore into the trusted-material seam.
func trustedMaterialOf(sigstore *ca.VirtualSigstore) trustedMaterialFunc {
	return func() (root.TrustedMaterial, error) { return sigstore, nil }
}

// TestNewVerifierIsStrictByDefault: the SCT relaxation exists only for tests
// using a virtual CA. Anything built for real use must require one, and the
// zero value must be strict so a Verifier can never be accidentally weakened.
func TestNewVerifierIsStrictByDefault(t *testing.T) {
	if NewVerifier().allowMissingSCT {
		t.Error("NewVerifier() produced a verifier that does not require an SCT")
	}
	if (&Verifier{}).allowMissingSCT {
		t.Error("the zero-value Verifier does not require an SCT")
	}
	if len(NewVerifier().verifierOptions()) != 3 {
		t.Error("the strict verifier is missing a verification requirement")
	}
}
