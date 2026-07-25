# Phase 0 Research: Distribution Channel Conformance

**Feature**: [spec.md](./spec.md) | **Date**: 2026-07-25

Five unknowns blocked the design: how a Go binary learns its own version without release-time
stamping; how to verify a Sigstore bundle in-process; how GoReleaser builds a multi-arch image
inside one invocation; what the MCP registry requires of `server.json` and its DNS namespace
proof; and how to tell who owns the running binary. Each is resolved below.

---

## 1. Version identity without release-time stamping

**Decision**: Resolve version through a three-tier fallback in a new `internal/buildinfo`
package — ldflags-stamped values first, then `runtime/debug.ReadBuildInfo()`, then an explicit
"unknown". `internal/cli` keeps its exported `Version`/`Commit`/`Date` vars as the ldflags
target so `.goreleaser.yaml` needs no change.

**Rationale**: `debug.ReadBuildInfo()` reports different things depending on how the binary was
produced, and the fallback must read each honestly:

| Build method | `Main.Version` | `vcs.*` settings |
| --- | --- | --- |
| `go install github.com/skaphos/sting/cmd/sting@v0.7.0` | `v0.7.0` | absent |
| `go install ...@latest` | resolved tag, e.g. `v0.7.0` | absent |
| `go build` inside the repo | `(devel)` | `vcs.revision`, `vcs.time`, `vcs.modified` present |
| `go build` outside a VCS tree | `(devel)` | absent |
| GoReleaser | ldflags win before any of this | — |

The module-proxy path is the one the README recommends and the one issue #121 calls out; it
yields a real version but no revision. The local-build path is the inverse. So the two sources
are complementary rather than ranked, and `Main.Version == "(devel)"` must be treated as *not a
version* — reporting `(devel)` to a user is the same defect as reporting `dev`.

`vcs.modified == "true"` is what satisfies FR-002's dirty-tree requirement.

**Alternatives considered**:

- *Keep ldflags as the only source, document `go install` as unsupported.* Rejected: the README
  actively recommends `go install`, and ADR-0001 makes a version-stamped `go install` a required
  channel, not a nicety.
- *Embed the version in a generated file at commit time.* Rejected: adds a release-time step that
  can drift from the tag, and solves nothing the build already records.

---

## 2. In-process Sigstore verification

**Decision**: Verify with `github.com/sigstore/sigstore-go`, pinning the certificate SAN to
sting's release workflow and the issuer to GitHub's OIDC provider.

**Rationale**: The release already produces exactly what this needs. `.goreleaser.yaml` signs the
checksum manifest keyless:

```yaml
signs:
  - cmd: cosign
    artifacts: checksum
    signature: "${artifact}.sigstore.json"
    args: [sign-blob, --bundle=${signature}, ${artifact}, --yes]
```

`release.yml` grants `id-token: write`, so `cosign sign-blob --yes` signs with the workflow's
ambient GitHub Actions OIDC identity, and `--bundle` emits a Sigstore bundle — the format
`sigstore-go` consumes directly. The verification chain is therefore:

1. Fetch `checksums.txt` and `checksums.txt.sigstore.json` for the target tag.
2. Verify the bundle over `checksums.txt`, pinned to the expected identity.
3. Look up the platform archive's digest in the verified manifest.
4. Verify the downloaded archive against that digest.

The relevant `sigstore-go` surface:

- Trusted root — `root.FetchTrustedRoot()` for the cached/embedded root, or
  `tuf.DefaultOptions()` → `tuf.New()` → `root.GetTrustedRoot(client)` for a live TUF fetch.
- Verifier — `verify.NewVerifier(trustedMaterial, verify.WithSignedCertificateTimestamps(1),
  verify.WithTransparencyLog(1), verify.WithObserverTimestamps(1))`.
- Identity policy — `verify.NewShortCertificateIdentity(issuer, "", "", sanRegexp)`.
- Artifact binding — `verify.NewPolicy(verify.WithArtifactDigest("sha256", digest),
  verify.WithCertificateIdentity(certID))`.

**Pinned identity**:

- Issuer: `https://token.actions.githubusercontent.com`
- SAN: `https://github.com/skaphos/sting/.github/workflows/release.yml@refs/tags/v*`

FR-010 exists because omitting the identity policy still *passes* verification for any
Sigstore-signed blob on earth. The pin is the whole control; the signature check alone is
decorative without it.

**Trust-root sourcing**: prefer the embedded/cached root so verification does not depend on a
second network service being reachable. A live TUF fetch is a fallback, never a silent one — if
neither is available the update refuses (FR-011), consistent with the spec's "refuses rather than
falling back to checksums alone" edge case.

**Alternatives considered**:

- *Shell out to `cosign`.* Rejected by clarification Q3. FR-012 makes verification unskippable, so
  a missing `cosign` becomes "cannot update" for essentially every end user.
- *Import `github.com/sigstore/cosign/v2`.* Rejected: it pulls the entire cosign command surface
  for one verification call. `sigstore-go` is the library extracted for exactly this use.
- *Verify GitHub's build-provenance attestation instead.* Viable and same trust root, but the
  cosign bundle is the artifact the release already publishes and points at the checksum
  manifest, which is the thing the archive digest must be checked against anyway.

**Cost**: this is the feature's one significant new dependency. Recorded in the plan's Complexity
Tracking against the constitution's minimize-and-justify constraint.

---

## 3. Multi-arch container image inside the single release invocation

**Decision**: Use GoReleaser's `dockers_v2` block. Add buildx and binfmt setup to `release.yml`
before the GoReleaser step, and a minimal `Dockerfile` that copies the already-built binary.

**Rationale**: `dockers_v2` is the current supported path — the classic `dockers:` +
`docker_manifests:` pair entered deprecation in v2.12, and sting pins `v2.17.0`, so `dockers_v2`
is available. It matters for FR-031 that it satisfies "same release invocation, from the same
binaries": `dockers_v2` builds a multi-platform manifest from binaries GoReleaser has *already*
built, rather than compiling inside the Dockerfile. GoReleaser's own framing: "GoReleaser already
builds your binaries (for all target platforms), so you don't need to build them again inside the
Dockerfile."

Relevant behavior:

- `platforms` defaults to `[linux/amd64, linux/arm64]` — exactly what FR-031 requires.
- The build context is laid out per platform (`linux/amd64/sting`, `linux/arm64/sting`), so the
  Dockerfile copies from `$TARGETPLATFORM` rather than building.
- SBOM generation is on by default; buildx also emits provenance attestations.
- `ids:` filters which build's binaries feed the image.
- Requires `docker buildx` and, for cross-arch emulation, `tonistiigi/binfmt`.

**Consequence for `release.yml`**: two setup steps (`docker/setup-buildx-action`, plus binfmt) and
a GHCR login before GoReleaser runs. A failure in any of them fails the release as a unit, which
is the FR-037 property clarification Q5 chose to preserve.

**Alternatives considered**:

- *Separate build-and-push job after the release.* Rejected by clarification Q5 — it would add a
  second channel that can half-land, weakening the one guarantee this feature is built around.
- *Classic `dockers:` + `docker_manifests:`.* Rejected: deprecated since v2.12, and it requires
  per-arch image definitions plus a manual manifest, which is more config for a worse outcome.

---

## 4. MCP registry entry and the `io.skaphos` namespace

**Decision**: Check in `server.json` declaring `io.skaphos/sting` with an OCI package entry
pointing at the GHCR image over stdio transport. Publish with `mcp-publisher` from `release.yml`,
authenticating by DNS with an Ed25519 key held as an organization secret.

**Rationale**: The registry derives namespace authority from the authentication method. Domain
authentication requires the server name to be the reverse-DNS form of a domain the publisher
controls — `skaphos.io` → `io.skaphos/*`, which is exactly the identity clarification Q2 chose.

The DNS proof is a TXT record at the domain apex:

```
skaphos.io. IN TXT "v=MCPv1; k=ed25519; p=<base64 public key>"
```

generated from an Ed25519 keypair. Publishing is then non-interactive, which is what makes it
CI-usable:

```sh
mcp-publisher login dns --domain skaphos.io --private-key "$MCP_REGISTRY_KEY"
mcp-publisher publish
```

`server.json` shape (schema `https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json`):
`name`, `description`, `repository`, `version`, and a `packages` array whose entries carry
`registryType` (`oci`), `identifier`, `version`, `transport.type` (`stdio`), and
`environmentVariables` with `name` / `description` / `isRequired` / `isSecret`. sting's
environment variables are `STING_TOKEN` and `STING_GITLAB_TOKEN`, both secret, neither strictly
required at startup — matching the `STING_`-prefixed viper binding in `internal/cli/root.go`.

**Key custody**: the registry also supports Google KMS and Azure Key Vault as signing backends
for the same DNS proof. A raw private key in a CI secret is the simplest thing that satisfies
FR-028's org-scoped requirement; a KMS-backed key is the upgrade path if key custody becomes a
concern, and needs no change to the namespace or the TXT record.

**Stability caveat**: the registry is explicitly in preview, with breaking changes and data resets
possible before general availability. This is the documented basis for the spec treating it as the
one channel exempt from failing the release as a unit (FR-029).

**Alternatives considered**:

- *`io.github.skaphos/sting` with GitHub OIDC auth.* The recommendation at clarification time —
  no DNS record, no secret, provable by the workflow's existing OIDC identity. Not chosen; the
  branded namespace was preferred for portability, and the added expiry surface is recorded in
  the spec's Dependencies and edge cases.
- *HTTP `/.well-known/mcp-registry-auth` proof.* Equivalent authority, but it makes publishing
  depend on a web host staying up, where DNS is already a dependency of the domain existing.

---

## 5. Install provenance detection

**Decision**: Resolve the executable to its real path (following symlinks), then classify it in
this order: Homebrew → system package database → Go toolchain → unmanaged.

**Rationale**: Order matters because the checks overlap. A Homebrew binary on `PATH` is a symlink
into a versioned cellar, so `os.Executable()` must go through `filepath.EvalSymlinks` before any
judgement — the spec's symlink edge case exists for precisely this.

| Owner | Detection | Upgrade command to print |
| --- | --- | --- |
| Homebrew | real path under the `brew --prefix` tree, or contains `/Cellar/` or `/Caskroom/` | `brew upgrade --cask sting` |
| RPM | `rpm -qf <path>` exits 0 | `sudo dnf upgrade sting` |
| dpkg | `dpkg -S <path>` exits 0 | download the new `.deb` and `sudo dpkg -i` (no apt repo — FR-025) |
| Go toolchain | path under `$GOBIN`, `$GOPATH/bin`, or `$HOME/go/bin` | `go install github.com/skaphos/sting/cmd/sting@latest` |
| Unmanaged | none of the above | proceed with self-update |

**On shelling out**: FR-009's "no external tooling" constraint governs *verification*, not
ownership detection. Consulting `rpm`/`dpkg` is reasonable because those binaries exist wherever
their package databases do; when absent, the check simply reports "not owned by this manager"
rather than failing. Homebrew detection prefers a path test over invoking `brew`, so it works
even when `brew` is not on `PATH`.

**Atomic replacement**: write the new binary to a temporary file in the *same directory* as the
target (so `os.Rename` stays within one filesystem and is atomic), `chmod` it to match, then
rename over the target. Where the platform forbids overwriting a running executable, rename the
running file aside first and clean it up on a later run — specified in FR-013, and gated off on
Windows per clarification Q4.

**Alternatives considered**:

- *Ask the user which channel they used.* Rejected: the spec's premise is that users do not
  remember, which is why FR-008 exists.
- *Write an install-provenance marker file at install time.* Rejected: sting does not control the
  Homebrew, rpm or dpkg install paths, so the marker would be missing in exactly the cases that
  matter, and it violates Principle VII by creating state that can diverge.

---

## 6. Release supply-chain coverage for the new artifacts

**Decision**: Add `.deb`/`.rpm` via `nfpms`, extend the SBOM block to cover packages, and let the
existing checksum + attestation steps carry the rest.

**Rationale**: Working out what each new artifact inherits for free versus what needs wiring:

- **Checksums** — GoReleaser's checksum step covers nfpm packages alongside archives, so FR-024's
  manifest requirement needs no extra configuration.
- **Provenance** — `release.yml` already attests with `subject-checksums: dist/checksums.txt`,
  which covers every artifact listed in that manifest. Packages therefore inherit provenance the
  moment they appear in it.
- **SBOM** — the current block is `artifacts: archive` only. Packages need a second entry
  (`artifacts: package`) to be covered.
- **Container image** — outside the checksum manifest entirely. It carries buildx-generated SBOM
  and provenance attestations, and needs its own signature. This is why SC-009 was corrected
  during clarification to state provenance appropriate to each artifact form rather than claiming
  the image sits in the checksum manifest.
- **macOS signing** — unchanged, and already correctly ordered: `notarize` runs after build and
  before archive, so packages and the image inherit the signed binary without per-channel work.

---

## 7. Post-release verification

**Decision**: A separate `verify` job in `release.yml`, gated on the release job, that queries each
channel and fails on a mismatch — with the MCP registry reported but non-blocking.

**Rationale**: Clarification Q1 requires confirmation by querying the channel rather than trusting
the publishing step. Per channel:

| Channel | Query | Blocking |
| --- | --- | --- |
| Release assets | `gh release view <tag>` — every expected archive, package, checksum, SBOM and bundle present | yes |
| Homebrew cask | fetch the cask file from `skaphos/homebrew-tools`, assert the version equals the tag | yes |
| Container image | `docker buildx imagetools inspect` the version tag, assert both platforms in the manifest list | yes |
| MCP registry | registry API lookup of `io.skaphos/sting`, assert the version equals the tag | no — warn only |

Version equality is the check, not reachability: a cask still pinned to the previous release
responds perfectly well, and is exactly the repokeeper ADR-0007 failure this guards against
(spec's "A channel publishes the wrong version" edge case).

Retries with backoff separate a genuine non-publish from third-party latency (FR-041) — registry
and tap propagation are not instantaneous.

**The `--skip=homebrew` hazard**: `release.yml` currently computes GoReleaser args and drops the
cask with only a `::warning::` if the app token cannot reach `skaphos/homebrew-tools`. Under
FR-038 a missing or unusable credential must fail the release rather than silently skipping the
channel, so that conditional needs to become a hard failure. This is the single highest-value
change in the feature relative to the failure ADR-0001 documents, and it is a change to *existing*
behavior rather than an addition.

---

## Sources

- [GoReleaser — Docker images v2](https://goreleaser.com/customization/package/dockers_v2/)
- [GoReleaser — Docker (deprecated in v2.12)](https://goreleaser.com/customization/docker/)
- [MCP Registry — Authentication](https://modelcontextprotocol.io/registry/authentication)
- [MCP Registry — `generic-server-json` reference](https://github.com/modelcontextprotocol/registry/blob/main/docs/reference/server-json/generic-server-json.md)
- [MCP Registry — Publishing quickstart](https://modelcontextprotocol.io/registry/quickstart)
- [sigstore-go](https://github.com/sigstore/sigstore-go) and its `docs/verification.md`
- `skaphos-resources` DECISIONS/0001; sting ADRs 0003, 0005, 0009, 0010; repokeeper ADR-0007
