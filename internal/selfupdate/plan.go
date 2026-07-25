// SPDX-License-Identifier: MIT

// Package selfupdate implements `sting update`.
//
// Three rules govern it, and they are the reason it exists in this shape rather
// than as a convenience wrapper around a download:
//
//   - Verify before replacing. The release publishes a cosign bundle and a
//     checksum manifest; both are checked, against a pinned signer identity,
//     before a byte is written. There is no way to skip this.
//   - Defer to the package manager. If another installer owns the binary,
//     sting prints that channel's upgrade command and exits non-zero rather
//     than overwriting a file it does not own.
//   - No implicit network calls. Nothing here runs except when the user asks
//     for it.
package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/skaphos/sting/internal/buildinfo"
)

// Action is what an update invocation has decided to do.
type Action int

const (
	// ActionReplace means proceed: download, verify, and swap the binary.
	ActionReplace Action = iota
	// ActionUpToDate means the running binary is already the target.
	ActionUpToDate
	// ActionDeferToManager means another installer owns this file.
	ActionDeferToManager
	// ActionGated means the platform's self-replacement path is disabled.
	ActionGated
	// ActionRefuse means the update cannot proceed safely.
	ActionRefuse
)

// Plan is the resolved intent of one invocation. `--check` computes a Plan and
// stops; a real run computes the same Plan and executes it, so the two can
// never diverge in their decision-making.
type Plan struct {
	Current     buildinfo.Info
	Target      string
	Provenance  Provenance
	AssetName   string
	DownloadURL string
	Action      Action
	Reason      string
}

// releaseVerifier proves a release was published by sting's release workflow
// and matches what was signed. It is an interface at the point of use so the
// download-and-replace path can be exercised without minting real signatures,
// while the policy itself is tested directly against sigstore.
type releaseVerifier interface {
	VerifyRelease(checksums, bundleJSON, artifact []byte, assetName string) error
}

// Updater resolves and applies updates. Every external dependency is a seam so
// the whole decision path is testable without a network, a package manager, or
// a particular operating system.
type Updater struct {
	Client   *Client
	Verifier releaseVerifier
	ExecPath func() (string, error)
	Run      commandRunner
	GOOS     string
}

// New returns an Updater wired to the real world.
func New() *Updater {
	return &Updater{
		Client:   NewClient(),
		Verifier: NewVerifier(),
		ExecPath: os.Executable,
		Run:      runCommand,
		GOOS:     runtime.GOOS,
	}
}

func (u *Updater) goos() string {
	if u.GOOS != "" {
		return u.GOOS
	}
	return runtime.GOOS
}

// Plan decides what to do. targetVersion is empty to mean "the latest stable
// release", or an explicit tag.
func (u *Updater) Plan(ctx context.Context, current buildinfo.Info, targetVersion string) (*Plan, error) {
	plan := &Plan{Current: current}

	// Refuse before touching the network when there is nothing to compare
	// against. Assuming a version-less build is out of date would mean
	// replacing a binary on a guess.
	if !current.Known() && targetVersion == "" {
		plan.Action = ActionRefuse
		plan.Reason = "this build records no version, so sting cannot tell whether it is " +
			"out of date. Re-run with an explicit target, e.g. sting update --version " +
			"v1.0.0"
		return plan, nil
	}

	release, err := u.resolveRelease(ctx, targetVersion)
	if err != nil {
		return nil, err
	}
	plan.Target = release.Tag

	// Ownership is checked before the platform gate so that a
	// package-managed binary gets the specific, actionable instruction
	// rather than the generic gate message.
	plan.Provenance = classify(u.execPath(), u.runner())

	switch {
	case plan.Provenance.Owner == OwnerUndeterminable:
		plan.Action = ActionRefuse
		plan.Reason = "sting cannot determine the path of the running binary, and will " +
			"not guess where to write"
		return plan, nil

	case plan.Provenance.Owner.Managed():
		plan.Action = ActionDeferToManager
		plan.Reason = fmt.Sprintf("sting was installed with %s and must be upgraded through it",
			plan.Provenance.Owner)
		return plan, nil

	case u.goos() == "windows":
		// Specified in full, deliberately not enabled: Windows release
		// binaries are not yet Authenticode-signed, and sting will not
		// ask a user to self-install an unsigned replacement.
		plan.Action = ActionGated
		plan.Reason = "self-update is not enabled on Windows: release binaries are not " +
			"yet Authenticode-signed"
		return plan, nil

	case sameVersion(current.Version, release.Tag):
		plan.Action = ActionUpToDate
		return plan, nil
	}

	plan.AssetName = assetNameFor(release.Tag, u.goos(), runtime.GOARCH)
	url, err := release.AssetURL(plan.AssetName)
	if err != nil {
		plan.Action = ActionRefuse
		plan.Reason = fmt.Sprintf("release %s has no asset for %s/%s. Available assets: %s",
			release.Tag, u.goos(), runtime.GOARCH, strings.Join(release.AssetNames(), ", "))
		return plan, nil
	}
	plan.DownloadURL = url
	plan.Action = ActionReplace
	return plan, nil
}

// resolveRelease fetches the target release, translating a missing release into
// a clear "not available yet" rather than an obscure error.
func (u *Updater) resolveRelease(ctx context.Context, targetVersion string) (*Release, error) {
	if targetVersion == "" {
		rel, err := u.Client.Latest(ctx)
		if err != nil {
			return nil, fmt.Errorf("resolving the latest release: %w", err)
		}
		return rel, nil
	}

	rel, err := u.Client.ByTag(ctx, targetVersion)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("release %s not found: %w", targetVersion, err)
		}
		return nil, fmt.Errorf("resolving release %s: %w", targetVersion, err)
	}
	return rel, nil
}

// Apply executes a plan whose Action is ActionReplace. It downloads the
// manifest, its signature, and the archive; verifies all of it; and only then
// writes anything.
func (u *Updater) Apply(ctx context.Context, plan *Plan) error {
	if plan.Action != ActionReplace {
		return fmt.Errorf("apply called for a plan that is not a replacement (action %d)", plan.Action)
	}

	release, err := u.Client.ByTag(ctx, plan.Target)
	if err != nil {
		return fmt.Errorf("re-resolving release %s: %w", plan.Target, err)
	}

	checksums, err := u.fetchAsset(ctx, release, checksumsName)
	if err != nil {
		return err
	}
	signature, err := u.fetchAsset(ctx, release, bundleName)
	if err != nil {
		return err
	}
	archive, err := u.Client.Download(ctx, plan.DownloadURL)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", plan.AssetName, err)
	}

	// Nothing has been written at this point, and nothing will be unless
	// every check below passes.
	if err := u.Verifier.VerifyRelease(checksums, signature, archive, plan.AssetName); err != nil {
		return err
	}

	binary, err := ExtractBinary(plan.AssetName, archive)
	if err != nil {
		return err
	}

	return Replace(plan.Provenance.RealPath, binary)
}

// fetchAsset retrieves a named release asset, distinguishing "the release is
// still publishing" from a verification problem.
func (u *Updater) fetchAsset(ctx context.Context, release *Release, name string) ([]byte, error) {
	url, err := release.AssetURL(name)
	if err != nil {
		return nil, fmt.Errorf("release %s is missing %s; it may still be publishing: %w",
			release.Tag, name, err)
	}
	data, err := u.Client.Download(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", name, err)
	}
	return data, nil
}

func (u *Updater) execPath() func() (string, error) {
	if u.ExecPath != nil {
		return u.ExecPath
	}
	return os.Executable
}

func (u *Updater) runner() commandRunner {
	if u.Run != nil {
		return u.Run
	}
	return runCommand
}

// sameVersion compares versions ignoring a leading "v", so a tag and a module
// version that differ only in that prefix are not mistaken for an upgrade.
func sameVersion(a, b string) bool {
	return strings.TrimPrefix(a, "v") == strings.TrimPrefix(b, "v")
}
