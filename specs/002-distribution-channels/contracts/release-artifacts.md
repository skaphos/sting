# Contract: Release artifact set and channel verification

**Feature**: [../spec.md](../spec.md) | **Date**: 2026-07-25

What one release must produce, and what post-release verification asserts about each channel
(FR-021–FR-024, FR-031–FR-035, FR-037–FR-041).

## Artifact set per release

`{V}` is the version without a leading `v`; `{v}` is the tag.

| # | Artifact | Count | New? | In checksum manifest? |
| --- | --- | --- | --- | --- |
| 1 | `sting_{V}_{os}_{arch}.tar.gz` (linux, darwin) | 4 | no | yes |
| 2 | `sting_{V}_windows_{arch}.zip` | 2 | no | yes |
| 3 | `sting_{V}_{arch}.deb` | 2 | **yes** | yes |
| 4 | `sting_{V}_{arch}.rpm` | 2 | **yes** | yes |
| 5 | `checksums.txt` | 1 | no | — |
| 6 | `checksums.txt.sigstore.json` (cosign bundle) | 1 | no | — |
| 7 | `*.sbom.json` per archive **and per package** | 10 | extended | yes |
| 8 | `ghcr.io/skaphos/sting:{V}` + `:latest`, multi-arch | 1 manifest | **yes** | **no** — see below |

**The image is deliberately not in the checksum manifest.** It carries buildx-generated SBOM and
provenance attestations plus its own signature, in the registry it is published to. SC-009 was
corrected during clarification to state provenance appropriate to each artifact *form* rather than
claiming a uniform manifest.

## Supply-chain coverage

What each new artifact inherits versus what needs wiring (research §6):

| Guarantee | Archives | `.deb` / `.rpm` | Image |
| --- | --- | --- | --- |
| Checksum manifest | existing | **inherited** — the checksum step covers nfpm packages | n/a |
| Build provenance attestation | existing | **inherited** — `subject-checksums: dist/checksums.txt` covers everything in the manifest | needs its own (buildx) |
| SBOM | existing (`artifacts: archive`) | **needs a second `sboms` entry** (`artifacts: package`) | buildx, on by default |
| cosign signature | over the manifest | over the manifest | needs image signing |
| macOS Developer ID + notarization | existing | **inherited** — `notarize` runs after build, before archive, so every downstream channel carries the signed binary | n/a |

The macOS row is the one that needs no work and is worth stating: signing happens early enough
that packages and the image get the signed binary for free.

## Single-invocation rule

Every channel except the MCP registry is produced by one GoReleaser invocation and fails the
release as a unit (FR-037). `dockers_v2` builds the multi-arch manifest from the binaries
GoReleaser has already built, so the image genuinely belongs to that invocation rather than
sitting beside it (clarification Q5).

Workflow prerequisites, all before the GoReleaser step, all release-blocking on failure:

- `docker/setup-buildx-action`
- binfmt (`tonistiigi/binfmt`) for cross-architecture emulation
- GHCR login

## Credential handling — a change to existing behavior

`release.yml` today computes GoReleaser args and, when the app token cannot reach
`skaphos/homebrew-tools`, appends `--skip=homebrew` with only a `::warning::`. The release then
completes green with the cask silently unpublished — the exact shape of the repokeeper ADR-0007
failure this feature exists to prevent.

**FR-038 makes this a hard failure.** A missing or unusable credential fails the release; it does
not skip the channel it belongs to. This is the single highest-value change in the feature
relative to the documented failure mode, and it modifies existing behavior rather than adding to
it.

## Post-release verification

A separate job, gated on the release job, asserting **the version each channel serves** — not that
it responds. A cask still pinned to the previous release responds perfectly well.

| Channel | Assertion | Blocking |
| --- | --- | --- |
| Release assets | every artifact in rows 1–7 present for `{v}` | yes |
| Homebrew cask | cask file in `skaphos/homebrew-tools` declares version `{V}` | yes |
| Container image | `imagetools inspect ghcr.io/skaphos/sting:{V}` lists both `linux/amd64` and `linux/arm64` | yes |
| MCP registry | `io.skaphos/sting` resolves at version `{V}` | **no** — warn only (FR-029) |

**Retries** with backoff before declaring a channel missing (FR-041), so third-party propagation
latency at the tap or registry is not reported as a failed release.

**Failure semantics**: any blocking channel mismatch fails the workflow (FR-040). The registry is
reported distinctly and does not invalidate the release. The published artifacts are never
unpublished by a verification failure — the job's job is to make a half-landed release *visible*,
which is what SC-010 measures.

## Container image

| Property | Value | Requirement |
| --- | --- | --- |
| Platforms | `linux/amd64`, `linux/arm64` | FR-031 |
| Tags | `{V}` and `latest` | FR-031 |
| Entrypoint | `sting mcp` — MCP server over stdio, no arguments needed | FR-032 |
| User | non-root | FR-033 |
| Contents | the GoReleaser-built binary plus CA certificates; no toolchain, no credentials | FR-033 |
| Configuration | `STING_`-prefixed environment variables, same names as the local binary | FR-034 |
| Provenance | SBOM, signature, build-provenance attestation | FR-035 |

The Dockerfile copies the already-built binary from the per-platform build context
(`linux/amd64/sting`, `linux/arm64/sting`) rather than compiling — GoReleaser has built every
target platform already, and rebuilding inside the image would break the "same binaries" property
FR-031 requires.

## Linux packages

| Property | Value | Requirement |
| --- | --- | --- |
| Formats | `deb`, `rpm` | FR-021 |
| Architectures | amd64, arm64 | FR-021 |
| Binary path | `/usr/bin/sting` | FR-022 |
| License / notices | packaging-conventional doc directory | FR-022 |
| Metadata | maintainer, homepage, license (MIT), description | FR-023 |
| Removal | leaves no files behind, package database consistent | User Story 3, scenario 3 |

**No hosted repository.** FR-025 requires the documentation to state this plainly, so a `.deb` on a
release page is not read as an implied apt repository — ADR-0001 records hosting signed apt/yum
repositories as explicitly out of scope, needing its own decision record.
