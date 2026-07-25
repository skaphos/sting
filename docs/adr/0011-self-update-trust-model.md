# 11. Self-update trust model

## Status

Accepted.

Implements the cross-cutting self-update requirement of
[skaphos-resources DECISIONS/0001 — Distribution channels by artifact shape](https://github.com/skaphos/skaphos-resources/blob/main/DECISIONS/0001-distribution-channels-by-artifact-shape.md).
Builds on [ADR 0009](0009-goreleaser-owns-the-github-release.md), which makes
GoReleaser the producer of the release artifacts this record consumes.

## Context

ADR-0001 requires every Shape 2 (end-user CLI) tool to ship a `<tool> update`
subcommand, subject to three rules: verify before replacing, defer to the
package manager, and make no implicit network calls.

A process that replaces its own binary is worth compromising. It runs with the
user's privileges, writes to a path on `PATH`, and the thing it writes is
executed next time the user runs the tool. The question this record answers is
not *whether* to verify — ADR-0001 settles that, noting a self-updater that
ignores available signing material "is strictly worse than no self-updater" —
but **what, concretely, is trusted, and by what mechanism**.

sting's releases already publish the material needed:

- `checksums.txt`, listing every archive and package;
- `checksums.txt.sigstore.json`, a cosign bundle signed over that manifest;
- per-archive SBOMs and a build-provenance attestation.

The signing is keyless. `.github/workflows/release.yml` grants `id-token:
write`, so `cosign sign-blob --yes` signs with the release workflow's ambient
GitHub Actions OIDC identity. The signature is therefore bound to *which
workflow in which repository at which ref* produced it.

## Decision

### 1. The trust anchor is a pinned signer identity, not merely a valid signature

Verification asserts both:

| | Value |
| --- | --- |
| OIDC issuer | `https://token.actions.githubusercontent.com` |
| Certificate SAN | `^https://github\.com/skaphos/sting/\.github/workflows/release\.yml@refs/tags/[^/]+$` |

**The pin is the entire control.** A Sigstore signature proves only that
*somebody* signed something and it was logged; without an identity constraint,
a bundle signed by any workflow in any repository on GitHub verifies
successfully. A verification step that omits the pin is decorative, and shipping
one would be worse than shipping none, because it reads as protection.

The SAN pattern is anchored at both ends and rejects a path segment after the
tag, so a fork, a different workflow in this repository, a branch ref, or a
lookalike host cannot match. `internal/selfupdate` carries a negative test for
each of those cases, asserted against the real sigstore policy object rather
than against a local approximation of it.

### 2. Verification happens in-process and cannot be skipped

sting verifies with `github.com/sigstore/sigstore-go`, embedded in the binary.
It requires no tooling on the user's machine.

There is **no** flag, environment variable, or configuration key that disables
any verification step, and none may be added. Verification order is fixed, each
step gating the next:

1. verify the cosign bundle over `checksums.txt`, with the identity policy
   applied as part of that verification rather than checked afterwards;
2. read the target artifact's digest from the now-trusted manifest;
3. hash the downloaded archive and compare.

Signed certificate timestamps, transparency-log inclusion, and observer
timestamps are all required. When the Sigstore trust root cannot be obtained,
the update **refuses** rather than falling back to checksums alone: an
unverified checksum proves only that a download matched what the same source
served.

### 3. sting never overwrites a file another installer owns

The running binary is resolved through symlinks first — Homebrew puts a link on
`PATH` pointing into a versioned cellar, so judging the link would misclassify a
package-managed install — then classified in order:

| Owner | Detected by | sting prints |
| --- | --- | --- |
| Homebrew | path under the brew prefix, `/Cellar/`, or `/Caskroom/` | `brew upgrade --cask sting` |
| RPM | `rpm -qf` claims the file | `sudo dnf upgrade sting` |
| dpkg | `dpkg -S` claims the file | download the release `.deb`, then `dpkg -i` |
| Go toolchain | path under `GOBIN`, `GOPATH/bin`, or `~/go/bin` | `go install …@latest` |
| Unmanaged | none of the above | *(sting replaces itself)* |

The Go toolchain is treated as an owning installer even though ADR-0001 names
only Homebrew, `rpm` and `dpkg`. The stated reasoning — that overwriting a file
another installer owns gets clobbered back — applies identically.

The dpkg advice deliberately does **not** say `apt upgrade`: there is no hosted
apt repository, and advice that cannot work is worse than none.

### 4. No implicit network calls, and no credentials

Only `sting update` contacts a release endpoint. No other command performs an
update check on any trigger or schedule.

The update path sends **no credential at all**. Release assets are public, and
sting authenticates with its own PATs by constitutional principle
([ADR 0002](0002-dedicated-pat-via-viper.md)) — borrowing an ambient
`GITHUB_TOKEN` here would be exactly the behavior that principle forbids. The
consequence is that the unauthenticated rate limit applies; exhausting it is a
reported failure mode, not a reason to authenticate.

### 5. Replacement is atomic

The new binary is written to a temporary file in the *same directory* as the
target, so the final rename stays within one filesystem and is atomic. An
interruption leaves either the complete old binary or the complete new one. For
the binary you are currently running, that is the difference between an
interrupted update and an unusable install.

### 6. Windows is specified but gated

The rename-aside mechanism for platforms that forbid overwriting a running
executable is implemented and tested. It is **disabled**: Windows release
binaries are not yet Authenticode-signed, and asking a user to self-install an
unsigned replacement is worse than not offering the feature. `sting update` on
Windows reports the available version, writes nothing, and exits non-zero,
naming the gate so it reads as deliberate rather than broken.

Lifting the gate is a policy change once Windows signing lands, not a new
design.

## Consequences

### Positive

- The signing material the release already publishes becomes load-bearing
  rather than decorative.
- Users get one upgrade instruction that is correct for their install channel,
  which matters because there are now six channels and users do not remember
  which one they used.
- Rollback is possible without leaving the tool (`--version`), under the same
  verification.

### Negative

- **The binary more than doubles: 11 MB → 25 MB (+122%), and `go.mod`
  requirements go from 44 to 106 modules.** `sigstore-go` transitively pulls in
  a large tree. Every user on every channel pays this cost to serve one command
  they may never run. This was accepted because the alternatives are worse:
  shelling out to `cosign` makes the command unusable for anyone without cosign
  installed (nearly every end user), and dropping verification is forbidden by
  ADR-0001. It is nonetheless a real regression in artifact size and should be
  revisited if `sigstore-go` grows a slimmer verification-only surface.
- **The dependency tree brings `golang.org/x/crypto/openpgp`**, which
  `govulncheck` reports as unmaintained (GO-2026-5932) with no fix available.
  It is not called by sting's code, so the CI gate passes, but it is now part of
  the module graph.
- **Self-update is an attack surface.** The verify-before-replace rule is what
  makes it acceptable, and it is load-bearing rather than decorative. Any change
  that weakens it needs a new record.
- **The identity pin is coupled to the repository path.** If sting moves
  repositories or renames its release workflow, previously shipped binaries pin
  an identity that no longer signs releases and will refuse to update. That
  failure is reported as an identity mismatch pointing at a manual install,
  rather than as a suspected tampering incident, but it is a real migration
  hazard.
- **Ownership detection shells out to `rpm` and `dpkg`.** Their absence is
  treated as "not owned by this manager" rather than an error, so the check is
  safe, but it is heuristic rather than authoritative.

### Neutral

- Verification requires network access for the Sigstore trust root, which is
  unremarkable for a command that is already downloading a release.
