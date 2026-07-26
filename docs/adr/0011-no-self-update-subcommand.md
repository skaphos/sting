# 11. No self-update subcommand (deviation from DECISIONS/0001)

## Status

Accepted.

Records a deliberate deviation from
[skaphos-resources DECISIONS/0001 — Distribution channels by artifact shape](https://github.com/skaphos/skaphos-resources/blob/main/DECISIONS/0001-distribution-channels-by-artifact-shape.md),
which lists a `<tool> update` subcommand as **required** for Shape 2 (end-user
CLI).

This record exists because that ADR requires it. Its "Deviating" section is
explicit: *"What does not deviate silently is dropping a **required** channel —
that needs a record in the repo that drops it."*

## Context

DECISIONS/0001 requires every Shape 2 tool to ship self-update, subject to three
rules: verify before replacing, defer to the package manager, and make no
implicit network calls. It also states that a self-updater ignoring available
signing material "is strictly worse than no self-updater" — verification is not
optional.

sting's releases publish everything verification needs: a `checksums.txt`
manifest and a cosign bundle signed over it, keyless, by the release workflow's
GitHub Actions OIDC identity.

A conforming implementation was built and worked. It verified the cosign bundle
in-process using `github.com/sigstore/sigstore-go`, pinned to sting's own
release-workflow certificate SAN and OIDC issuer; classified install provenance
(Homebrew, rpm, dpkg, Go toolchain) and refused to overwrite files it did not
own; replaced the binary atomically; and contacted the network only when
explicitly invoked.

**The cost was the problem.** Verifying in-process is the only way to satisfy
both rules at once — verification cannot be skipped, and requiring the user to
have `cosign` installed would make the command unusable for nearly every end
user. But `sigstore-go` transitively pulls a large dependency tree:

| | Before | With self-update |
| --- | --- | --- |
| Binary size (`-s -w`) | 11 MB | 25 MB (**+122%**) |
| `go.mod` requirements | 44 | 106 |
| Unmaintained modules in the graph | 0 | 1 (`golang.org/x/crypto/openpgp`, GO-2026-5932, no fix available) |

Every user on every channel pays that cost — in download size, in image size, in
supply-chain surface — to serve one command many will never run. sting is a
small, single-purpose evidence tool; more than doubling it for a convenience
feature inverts the tool's own proportions.

## Decision

**sting does not ship a `sting update` subcommand.** Upgrades run through the
channel the user installed from.

This is a deviation from a required channel, taken knowingly:

- The implementation is not weakened to reduce its cost. There is no
  checksum-only mode, no "verify if cosign is present" fallback, and no
  unpinned-identity shortcut. DECISIONS/0001 is right that a self-updater which
  ignores available signing material is worse than none, so the choice is
  between a correct self-updater at +122% binary size and no self-updater. We
  chose the latter.
- The gap is closed by documentation rather than code. `README.md` carries an
  upgrade path for every channel sting publishes to, so a user who does not
  remember how they installed sting can still find the right command.
- Version identity is unaffected and still shipped. DECISIONS/0001 lists
  "`go install` produces a version-stamped binary" as its own required item, and
  `internal/buildinfo` satisfies it. It was originally motivated in part by
  self-update needing to know its own version; that motivation is gone, the
  requirement is not.

### What users get instead

| Installed with | Upgrade with |
| --- | --- |
| Homebrew | `brew upgrade --cask sting` |
| `.rpm` | `sudo dnf upgrade sting` |
| `.deb` | download the next release's `.deb`, then `sudo dpkg -i` |
| `go install` | `go install github.com/skaphos/sting/cmd/sting@latest` |
| container image | `docker pull ghcr.io/skaphos/sting:latest` |
| downloaded archive | download the next release's archive |

Every other channel DECISIONS/0001 requires of sting is shipped: release
archives with SBOM, cosign signature and provenance; Linux `.deb`/`.rpm`; the
Homebrew cask; macOS signing and notarization; the MCP registry entry; and a
multi-arch container image.

## Consequences

### Positive

- sting stays an 11 MB binary with a 44-module dependency graph. For a tool
  whose value is being a small, auditable primitive, that is not a cosmetic
  property.
- No unmaintained cryptography enters the module graph.
- No self-replacing code path exists, so the attack surface DECISIONS/0001
  describes — "a process that replaces its own binary is worth compromising" —
  is absent rather than mitigated.

### Negative

- **sting is measurably non-conforming against an accepted org standard.**
  DECISIONS/0001 names sting as one of the first adoption targets, and this is
  the one required channel it does not meet.
- **Users must know how they installed sting to upgrade it.** This is precisely
  the problem self-update solves, and it gets worse as the channel count grows —
  sting now publishes to six. The README table is a weaker answer than a command
  that works it out.
- **The decision is cost-driven, not principled.** If `sigstore-go` grows a
  slimmer verification-only surface, or another verification path lands with a
  materially smaller footprint, the reason for this deviation disappears and it
  should be revisited.

### Follow-up

DECISIONS/0001's deviation process asks for a pull request against that document
explaining the divergence, so it can either become the standard or be driven
back into the standard shape. That is a change to `skaphos-resources`, not to
this repository, and is not part of this record. The specific question worth
putting upstream is whether "required" should be conditional on the
verification cost for the tool in question, since repokeeper and oiax will hit
the same tradeoff.
