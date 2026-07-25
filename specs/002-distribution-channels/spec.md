# Feature Specification: Distribution Channel Conformance

**Feature Branch**: `feature/distribution-channels`

**Created**: 2026-07-25

**Status**: Draft

**Input**: User description: "https://github.com/skaphos/sting/issues/121 — Conform to DECISIONS/0001: distribution channels by artifact shape. Note that we'll do implementation on the same PR as the specification."

## Problem

`skaphos-resources` [DECISIONS/0001 — Distribution channels by artifact shape][adr0001] (Accepted,
2026-07-25) makes a tool's distribution channels a function of what the artifact *is*. sting is
both an end-user CLI (Shape 2) and an MCP server (Shape 3), so it inherits the union of both
channel sets — the most heavily loaded repository under that record. sting and repokeeper are
named as the first adoption targets.

sting already conforms on four channels: GitHub release archives with checksums, per-archive
SBOM, cosign bundle and build provenance; a Homebrew cask into `skaphos/homebrew-tools`; macOS
Developer ID signing and notarization; and the in-binary MCP runtime installer established by
[ADR 0003][adr0003]. Five required obligations are unmet, and each one is a user who cannot
install, cannot upgrade, or cannot find the tool:

1. **Linux users have no package.** Every Linux install is "download a tarball, extract it, put
   it on `PATH` yourself." No `.deb`, no `.rpm`.
2. **Anyone installing the way the README recommends gets a binary that does not know its own
   version.** `internal/cli/version.go:14` defaults `Version` to `"dev"` with no build-metadata
   fallback, so `go install github.com/skaphos/sting/cmd/sting@latest` followed by
   `sting version` prints `sting dev`. Release-time ldflags are the only source of version
   information. A user cannot tell what they are running, and neither can a bug report.
3. **There is no upgrade path the tool itself can offer.** Every upgrade runs through whichever
   channel the user originally installed from, and sting cannot tell them which one that was.
4. **sting is invisible to MCP clients.** It is an MCP server with no entry in the index those
   clients read.
5. **sting cannot be run as a container.** A `docker`-based MCP client configuration — the
   portable way to run a server without a local toolchain — has nothing to point at.

Closing these is the work. The constraint that shapes *how* is recorded in the ADR's negative
consequences and in [repokeeper ADR-0007][rk0007]: every channel added multiplies the
silent-staleness surface. repokeeper's cask sat pinned at `0.6.0` across two releases because a
release run died before the tap step and nothing said so. Going from four channels to nine means
nine places a release can half-land. The mitigation is that all of them are fed by one release
invocation that fails as a unit — and where a channel cannot be held to that (the MCP registry),
its outcome must be *reported*, not assumed.

[adr0001]: https://github.com/skaphos/skaphos-resources/blob/main/DECISIONS/0001-distribution-channels-by-artifact-shape.md
[adr0003]: ../../docs/adr/0003-multi-runtime-installer-and-readonly-safety.md
[rk0007]: https://github.com/skaphos/repokeeper/blob/main/docs/adr/0007-release-binaries-and-homebrew.md

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Know which version you are running, however you installed it (Priority: P1)

A developer who installed sting from source — the path the README offers as the alternative to
Homebrew — runs the version command and gets the actual version, revision and build time of the
binary in their hand, not a placeholder. The same is true of a colleague's release build, a CI
build, and a locally compiled working copy; each reports what it honestly knows.

**Why this priority**: It is the smallest change with the widest blast radius. Today every
source install is indistinguishable from every other source install, which makes a bug report
unactionable and makes self-update impossible — a tool that cannot state its own version cannot
decide whether it is out of date. Every other story here is either unblocked by this one or
unaffected by it.

**Independent Test**: Build the binary without any release-time version stamping, run the
version command, and confirm it reports the module version and revision recorded in the build
rather than a placeholder. Then build it *with* release-time stamping and confirm the stamped
values are what appear.

**Acceptance Scenarios**:

1. **Given** a binary installed directly from the module at a published version, **When** the
   user asks for the version, **Then** the published version and the revision it was built from
   are reported, and no placeholder value appears.
2. **Given** a release binary produced by the release pipeline, **When** the user asks for the
   version, **Then** the values the pipeline stamped in are reported unchanged — release output
   is what it is today.
3. **Given** a binary built from a working copy with uncommitted changes, **When** the user asks
   for the version, **Then** the report identifies it as a modified build rather than presenting
   it as a clean build of the underlying revision.
4. **Given** a binary built in a way that records no version information at all, **When** the
   user asks for the version, **Then** the report says the information is unavailable rather
   than inventing a value.

---

### User Story 2 - Upgrade in place, or be told exactly how to (Priority: P2)

A developer running an out-of-date sting asks it to update. If the running binary is one the
developer installed by hand, sting verifies the new release was published by the project and is
intact, then replaces itself. If the binary belongs to a package manager, sting refuses to touch
it and prints the one command that will do the job correctly for that channel.

**Why this priority**: It is the channel that makes every other channel survivable. With nine
install paths, "how do I upgrade?" has nine answers and the user cannot be expected to remember
which one applies to them. It also carries the highest risk in this feature — a process that
replaces its own binary is worth compromising — so it is specified in full and gated on
verification rather than shipped as a convenience.

**Independent Test**: Point the update command at a release whose signature or checksum has been
tampered with and confirm the installed binary is untouched and the exit status is non-zero.
Then place a binary under a simulated package-manager-owned location and confirm the update
command prints that channel's upgrade command and changes nothing.

**Acceptance Scenarios**:

1. **Given** a hand-installed binary older than the latest release, **When** the user runs the
   update command, **Then** sting verifies the publisher signature over the release checksum
   manifest and the individual artifact's checksum, replaces the binary, and reports the version
   it moved from and to.
2. **Given** a downloaded artifact whose signature or checksum does not match, **When** the
   update runs, **Then** the existing binary is left unchanged, the failure names which check
   failed, and the command exits non-zero.
3. **Given** a binary installed by a package manager — a Homebrew prefix, or a file owned by the
   system package database, or one placed by the language toolchain — **When** the user runs the
   update command, **Then** sting prints the correct upgrade command for that channel, exits
   non-zero, and modifies nothing.
4. **Given** a binary already at the latest released version, **When** the user runs the update
   command, **Then** sting says so and exits successfully without downloading anything.
5. **Given** any command other than the update command, **When** it runs, **Then** no request is
   made to any release or version endpoint — update checks happen only when explicitly asked
   for.
6. **Given** the update is interrupted partway through replacing the binary, **When** the user
   next runs sting, **Then** they have either the complete old binary or the complete new one,
   never a truncated file.
7. **Given** a user without permission to write to the binary's location, **When** the update
   runs, **Then** the failure names the path and the permission problem and suggests the
   correct action, and sting does not attempt to escalate privileges.
8. **Given** a release that has proven bad, **When** the user asks to update to a named earlier
   version, **Then** that version is verified and installed by the same rules, so rollback is
   possible without leaving the tool.

---

### User Story 3 - Install on Linux with the system package manager (Priority: P3)

A developer on a Debian- or RPM-based Linux distribution downloads a package from the release
page and installs it with their system package manager in one command, getting the binary on
`PATH` and the license and notice files where their distribution expects them.

**Why this priority**: It is the largest population currently served worst — every Linux user
today does manual tarball placement — and it is the lowest-risk item in the feature, being
additional output from a release run that already produces the binaries. It is independent of
every other story.

**Independent Test**: Take the packages from a release run and install each on a matching
container image, then confirm the binary runs, reports the release version, and is registered
with the package database; then remove it and confirm nothing is left behind.

**Acceptance Scenarios**:

1. **Given** a published release, **When** a user looks at its assets, **Then** there is a
   `.deb` and an `.rpm` for both 64-bit Intel and 64-bit ARM, alongside the existing archives.
2. **Given** one of those packages, **When** it is installed with the system package manager,
   **Then** the binary is on `PATH`, reports the release version, and the license and third-party
   notice files are installed to the location the distribution expects.
3. **Given** an installed package, **When** the user removes it, **Then** every file it placed is
   removed and the package database is left consistent.
4. **Given** a published release, **When** a user verifies its assets, **Then** the packages are
   listed in the same checksum manifest and covered by the same signature and provenance
   attestation as every other artifact in that release.
5. **Given** a user reading the install documentation, **When** they reach the Linux packages,
   **Then** the documentation states plainly that there is no hosted package repository and that
   upgrades mean downloading the next release's package.

---

### User Story 4 - Find sting from an MCP client (Priority: P4)

Someone looking for a commit-evidence MCP server finds sting in the registry their client reads,
sees what it does and what it needs, and installs it — instead of only ever finding it by already
knowing it exists.

**Why this priority**: Discoverability is the whole point of the channel, but it reaches users who
have not yet found sting rather than users blocked today, and the ADR itself notes this is the
least settled channel with the most movement under it. It ranks below the paths that unblock
existing users.

**Independent Test**: Validate the checked-in server description against the registry's published
schema in CI, then confirm a release run publishes an entry whose version matches the release and
whose reported outcome — published or not — appears in the release run's output.

**Acceptance Scenarios**:

1. **Given** the repository, **When** it is inspected, **Then** it carries a checked-in server
   description naming the server, what it does, how it is run, and what configuration it needs,
   and that file is schema-validated on every change.
2. **Given** a published release, **When** the registry is queried, **Then** sting is present at
   the released version.
3. **Given** a release where the registry publish fails, **When** the release run finishes,
   **Then** the failure is surfaced in the run's output rather than passing silently, the rest of
   the release is still valid and complete, and the publish can be retried without cutting a new
   release.
4. **Given** the server description, **When** the server's advertised capabilities change,
   **Then** the description is updated in the same change, so the registry never describes a
   surface sting does not have.

---

### User Story 5 - Run sting as a container from an MCP client config (Priority: P5)

A developer configures their MCP client to run sting from a container image and gets a working
server on Intel or ARM, with no Go toolchain, no local install, and credentials supplied by the
client's environment rather than baked into the image.

**Why this priority**: It removes the local-install prerequisite entirely and is required for
Shape 3, but it serves the same discovery-stage user as Story 4 and is the furthest from a user
who is blocked right now.

**Independent Test**: Pull the published image on both architectures and drive the containerized
server with an MCP client over its standard transport, confirming a read-only commit query
succeeds with a credential passed through the environment.

**Acceptance Scenarios**:

1. **Given** a published release, **When** the container registry is queried, **Then** an image
   exists at the release version and at a moving current tag, working on both 64-bit Intel and
   64-bit ARM.
2. **Given** that image, **When** it is run with no arguments, **Then** it starts the MCP server
   on its standard transport, ready for a client to connect.
3. **Given** that image, **When** a credential is supplied through the environment, **Then**
   queries authenticate with it; and **When** none is supplied, **Then** the failure states that
   a credential is required and how to supply it.
4. **Given** that image, **When** it is inspected, **Then** it runs as an unprivileged user,
   contains no credentials, and carries the same SBOM, signature and provenance guarantees as
   the release archives.
5. **Given** the documentation, **When** a user reaches the MCP setup section, **Then** a
   container-based client configuration is shown alongside the existing local-binary one.

---

### Edge Cases

- **Update from a build with no version information.** A build that reports an unknown version
  cannot be compared against the latest release; the update command must say so and require an
  explicit target version rather than guessing.
- **Update on a platform with no matching release asset.** The command names the platform and the
  assets that do exist rather than failing obscurely.
- **Update while the release is still publishing.** A tag exists but its assets or checksum
  manifest do not yet; treated as "not available", not as a verification failure.
- **Draft and pre-release versions.** Never selected as "latest"; reachable only by naming them
  explicitly.
- **Replacing a running executable on Windows.** The platform will not permit overwriting a file
  in use; the update must still leave the user with exactly one complete, working binary.
- **A binary reached through a symlink** — including a Homebrew-style link into a versioned
  cellar — resolves to its real path before ownership is judged, so package-managed installs are
  not mistaken for hand-installed ones.
- **A binary whose location cannot be determined at all.** sting refuses to write rather than
  guessing a path.
- **Signature material present but unverifiable offline.** Verification requires network access;
  when it cannot complete, the update refuses rather than falling back to checksums alone.
- **Package install over an existing hand-placed binary at the same path.** The package manager's
  own conflict behavior governs; sting does not work around it, and the documentation says which
  wins.
- **Release signing secrets unset.** The macOS notarization block is gated on the secrets being
  set and silently skips when they are not, shipping unsigned binaries into every downstream
  channel. Each channel added widens that exposure, so this feature must not make an unsigned
  release *harder* to notice than it already is.
- **Registry entry drifts from the release.** A registry entry naming a version that was never
  published, or missing the current one, is a stale-channel failure of the kind
  repokeeper ADR-0007 records — the release run's reported outcome is what makes it visible.

## Requirements *(mandatory)*

### Functional Requirements

**Version identity**

- **FR-001**: The version command MUST report a meaningful version for a binary built without
  release-time stamping, derived from the build's own recorded module and revision metadata.
- **FR-002**: The version command MUST report the source revision and, where recorded, the build
  time and whether the working tree was modified.
- **FR-003**: Release-time stamped values MUST take precedence over build-recorded metadata, so
  released binaries report exactly what they report today.
- **FR-004**: When neither stamped values nor build metadata are available, the version command
  MUST report the information as unavailable and MUST NOT substitute a value that could be
  mistaken for a real version.
- **FR-005**: The resolved version MUST be available to the rest of the tool as a single value,
  so the update path and the version command can never disagree.

**Self-update**

- **FR-006**: sting MUST provide an update command that upgrades the running binary to a
  published release.
- **FR-007**: No command other than the update command MAY contact a release or version endpoint,
  for any purpose, including background or opportunistic update checks.
- **FR-008**: The update command MUST determine whether the running binary is managed by another
  installer — a Homebrew prefix, a system package database entry, or a language-toolchain install
  location — and when it is, MUST print that channel's correct upgrade command, exit non-zero,
  and modify nothing.
- **FR-009**: Before replacing anything, the update MUST verify the project's publisher signature
  over the release's checksum manifest, and MUST verify the downloaded artifact against that
  manifest.
- **FR-010**: The update MUST refuse and leave the installed binary unchanged if any verification
  step fails, and MUST state which check failed.
- **FR-011**: Verification MUST NOT be skippable by a flag, an environment variable, or any other
  user-facing switch.
- **FR-012**: The binary MUST be replaced atomically, such that an interruption at any point
  leaves either the complete previous binary or the complete new one in place.
- **FR-013**: The update command MUST offer a mode that reports what it would do — current
  version, target version, install ownership, and outcome — without downloading or writing
  anything.
- **FR-014**: A successful update MUST report the version it moved from and the version it moved
  to.
- **FR-015**: The update command MUST accept an explicit target version, so a user can move to a
  known-good earlier release without leaving the tool.
- **FR-016**: When the running version cannot be determined, the update MUST require an explicit
  target version rather than assuming the binary is out of date.
- **FR-017**: When replacement fails for want of filesystem permission, the failure MUST name the
  path and the reason and suggest the correct action, and sting MUST NOT attempt to escalate
  privileges.
- **FR-018**: The update path MUST NOT transmit any user, host, or installation identifying
  information beyond what fetching public release assets inherently requires.
- **FR-019**: Draft and pre-release versions MUST NOT be selected as the update target unless
  named explicitly.

**Linux OS packages**

- **FR-020**: Each release MUST publish `.deb` and `.rpm` packages for 64-bit Intel and 64-bit
  ARM, built from the same binaries as that release's archives.
- **FR-021**: Packages MUST install the binary onto the system `PATH` and MUST install the
  license and third-party notice files to the location their packaging convention expects.
- **FR-022**: Packages MUST declare complete metadata — maintainer, homepage, license, and
  description — consistent with the repository's other published artifacts.
- **FR-023**: Packages MUST appear in the release's checksum manifest and MUST be covered by the
  same signature and build-provenance attestation as every other artifact in that release.
- **FR-024**: Installation documentation MUST state that no hosted package repository exists, so
  a `.deb` on a release page is not read as an implied apt repository.

**MCP registry**

- **FR-025**: The repository MUST carry a checked-in server description that names the server,
  describes what it does, and states how it is run and what configuration it requires.
- **FR-026**: That description MUST be validated against the registry's published schema in CI,
  so an invalid entry fails before a release rather than at publish time.
- **FR-027**: Each release MUST publish the entry to the MCP registry at the released version.
- **FR-028**: A registry publish failure MUST be surfaced in the release run's output and MUST
  NOT be silent; it MUST NOT invalidate the remainder of the release, and it MUST be retryable
  without cutting a new release.
- **FR-029**: The description MUST be updated in the same change as any change to the server's
  advertised capabilities.

**Container image**

- **FR-030**: Each release MUST publish a container image for 64-bit Intel and 64-bit ARM Linux,
  tagged with the release version and with a moving current tag.
- **FR-031**: The image's default behavior when run with no arguments MUST be to start the MCP
  server on its standard transport.
- **FR-032**: The image MUST run as an unprivileged user and MUST contain no credentials or
  configuration specific to any user or organization.
- **FR-033**: The image MUST accept credentials and configuration through the environment, using
  the same names as the local binary.
- **FR-034**: The image MUST carry an SBOM, a signature, and a build-provenance attestation
  equivalent to those on the release archives.
- **FR-035**: Documentation MUST show a container-based MCP client configuration alongside the
  existing local-binary configuration.

**Pipeline coherence and scope discipline**

- **FR-036**: Every channel except the MCP registry MUST be produced by the single release
  invocation, so a failure in any one of them fails the release as a unit rather than half-landing
  it.
- **FR-037**: The release run MUST report which channels published and which did not, so a
  partially-landed release is visible without a user reporting it.
- **FR-038**: No requirement in this feature may introduce a code path that writes to, or is
  capable of writing to, any provider-side object; the read-only invariant and the derived
  auto-approve list remain unchanged.
- **FR-039**: Installation and upgrade documentation MUST be updated for every channel this
  feature adds, and MUST make clear which upgrade path applies to which install path.

### Key Entities

- **Version identity**: what a running binary knows about itself — version, source revision,
  build time, and whether the working tree was clean. Sourced from release-time stamping when
  present and from the build's own recorded metadata otherwise.
- **Install provenance**: how the running binary got where it is — hand-placed, package-manager
  owned, toolchain-installed, or unknown. Determines whether sting may replace it and, if not,
  which upgrade instruction is correct.
- **Release artifact set**: everything one release publishes — archives, Linux packages, the
  container image, the checksum manifest, SBOMs, signatures and attestations — plus which of them
  actually landed.
- **Server description**: the checked-in declaration of sting's MCP identity, capabilities,
  runtime, and configuration, published to the registry and kept in step with the released
  version.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A binary installed from source reports a real version and revision; the placeholder
  value that ships today appears in zero installation paths that produce a usable binary.
- **SC-002**: A user on a mainstream Debian- or RPM-based distribution goes from the release page
  to a working `sting` on `PATH` in a single package-manager command, on both 64-bit Intel and
  64-bit ARM.
- **SC-003**: In 100% of update attempts where the downloaded artifact fails signature or checksum
  verification, the installed binary is unchanged and the command exits non-zero. There is no
  supported way to bypass verification.
- **SC-004**: In 100% of update attempts against a package-manager-owned binary, sting prints that
  channel's correct upgrade command and modifies no file.
- **SC-005**: Across sting's entire command surface, zero requests to release or version endpoints
  are issued by any command other than the update command.
- **SC-006**: An interrupted update leaves a complete, runnable binary in 100% of trials, across
  every supported platform.
- **SC-007**: sting is resolvable by name in the MCP registry, at the current released version,
  within one release cycle of this change landing.
- **SC-008**: An MCP client configured to run sting from the published container image completes
  a read-only commit query on both supported architectures with no local Go toolchain present.
- **SC-009**: Every artifact in a release — archives, packages, and image — is covered by the
  release's checksum manifest, signature and provenance attestation. Zero unsigned or unattested
  artifacts ship.
- **SC-010**: When any channel fails to publish, the release run's output names it. Zero releases
  complete as apparently successful while a channel is missing.
- **SC-011**: A user can move to a specific named earlier release through the update command,
  with the same verification applied.

## Assumptions

- **Windows package managers are out of scope for this feature.** `winget` and `scoop` are
  *recommended*, not required, under ADR-0001. The issue's follow-up comment records that Windows
  Authenticode signing is "intended, not yet required" and that pushing an unsigned binary into
  those channels is worse than not pushing it, and recommends sequencing Windows signing ahead of
  them. They are therefore deferred to separate work behind Authenticode signing, rather than
  shipped alongside these five channels. This is a scope decision taken from the issue's own
  guidance, not a silent drop of a required channel.
- **Binaries installed by the Go toolchain are treated as externally managed** for the purposes of
  the update command, in the same class as Homebrew and system packages: sting prints the
  toolchain's own upgrade command rather than overwriting a file the toolchain placed. The ADR
  names Homebrew, `rpm` and `dpkg` explicitly; extending the same rule to the toolchain follows
  its stated reasoning that overwriting a file another installer owns gets clobbered back.
- **The MCP registry is the one channel exempt from failing the release as a unit.** ADR-0001
  states the registry is the least settled channel and that a break there is "a smaller emergency
  than a broken cask." It is therefore reported-on-failure rather than release-failing, unlike
  every other channel here.
- **The existing macOS signing and notarization arrangement is unchanged by this feature.** sting
  already conforms and is one of the ADR's two reference implementations. The known gap — that the
  notarization block silently skips when its secrets are unset — is noted as an edge case above
  and is separate follow-up work, not part of this scope.
- **Promoting the `MACOS_*` secrets from repository to organization scope is out of scope here.**
  ADR-0001 calls for it; it is an org-administration change tracked in `skaphos-resources`.
- **Release infrastructure stays as it is.** The release remains tag-triggered, driven by a single
  GoReleaser invocation, with release notes owned per [ADR 0009][adr0009] and version bumps per
  [ADR 0005][adr0005]. This feature adds outputs to that run; it does not restructure it.
- **The container image serves the MCP server use case.** It exists because Shape 3 requires a
  `docker`-runnable server; it is not positioned as a general-purpose way to run the CLI.
- **Specification and implementation land on the same pull request**, per the request that
  produced this spec. The spec, plan, tasks and implementation are one reviewable change against
  `feature/distribution-channels`, closing issue #121.

## Out of Scope

- Hosted, signed apt or yum repositories. ADR-0001 excludes them explicitly; they would need
  their own decision record.
- AUR packaging and a Nix flake. Optional under ADR-0001 and not expected.
- Windows Authenticode signing, and the `winget` and `scoop` channels that depend on it.
- Homebrew core, Chocolatey, Snap and Flatpak — all deliberately excluded by ADR-0001.
- Removing the Homebrew cask's quarantine hook. ADR-0001 keeps it, because a notarization ticket
  cannot be stapled to a bare binary in a tarball, and says removing it is a separate call backed
  by testing on real hardware.
- Any change to sting's query behavior, MCP tool surface, configuration precedence, or public
  package contract.

## Dependencies

- **ADR-0001 (`skaphos-resources`)** — the standard being adopted; defines the required channel
  set per shape and the three self-update rules.
- **Existing release supply chain** — checksum manifest, per-archive SBOM, cosign signature and
  build-provenance attestation. Every new channel extends these rather than sitting beside them.
- **`skaphos/homebrew-tools` and the release bot credentials** — unchanged, but they share the
  release run whose failure semantics this feature tightens.
- **The MCP registry's publishing interface and schema** — external, and the least stable
  dependency in this feature, which is why its failure mode is specified separately.
- **GitHub Container Registry** — the publish target for the container image.

## Constitution Alignment

- **I. Read-Only by Design** — nothing here adds a provider-mutating path. The update command
  writes only to sting's own binary on the local filesystem (FR-038).
- **II. Evidence-Grade, Explainable Output** — the version command and the update command must
  both say what they know and why they refused; "failed" without a reason is a defect
  (FR-004, FR-010, FR-017).
- **III. Deterministic, Reconstructible** — verification is mandatory and unskippable, and update
  behavior is a function of the release contents and the install location, not of ambient state
  (FR-009, FR-011).
- **IV. Explicit Configuration** — no implicit network calls and no ambient telemetry
  (FR-007, FR-018).
- **VII. Never a Second Source of Truth** — the binary is replaced atomically, and files owned by
  another installer are never overwritten (FR-008, FR-012).
- **VIII. Technical Precision, Honest Scope** — the documentation must state that shipping a
  `.deb` is not an apt repository, and that Windows binaries remain unsigned
  (FR-024, FR-039).
- **Testing (non-negotiable)** — new behavior ships with direct test coverage in the same change,
  tests do not touch the network, and tests that touch the filesystem isolate `HOME` and
  `USERPROFILE`. The per-package coverage gate applies.

[adr0005]: ../../docs/adr/0005-release-please-owns-release-notes.md
[adr0009]: ../../docs/adr/0009-goreleaser-owns-the-github-release.md
