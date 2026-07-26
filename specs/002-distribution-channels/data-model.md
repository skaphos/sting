# Data Model: Distribution Channel Conformance

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-07-25

The spec names four entities. None of them is persisted — sting adds no durable state in this
feature (Principle VII). They are in-memory values resolved at command time, plus one checked-in
declaration and one release-time artifact set.

---

## 1. Version identity

What a running binary knows about itself. Resolved once at startup by `internal/buildinfo`.

```go
// Package internal/buildinfo
type Info struct {
    Version  string  // "v0.7.0", or empty when unknown
    Revision string  // full VCS revision, or empty
    Time     string  // RFC3339 build time, or empty
    Modified bool    // built from a dirty working tree
    Source   Source  // where the above came from
}

type Source int
const (
    SourceUnknown Source = iota // nothing usable was recorded  (FR-004)
    SourceBuildInfo             // runtime/debug.ReadBuildInfo   (FR-001)
    SourceLDFlags               // stamped at release time       (FR-003)
)
```

**Resolution rules** (FR-001–FR-005):

| Precedence | Condition | Result |
| --- | --- | --- |
| 1 | ldflags `Version` set and not the `"dev"` sentinel | `SourceLDFlags`; use stamped values verbatim |
| 2 | `debug.ReadBuildInfo()` gives `Main.Version` other than `""` or `"(devel)"` | `SourceBuildInfo`; version from the module, revision/time from `vcs.*` if present |
| 3 | `Main.Version` is `"(devel)"` but `vcs.revision` is present | `SourceBuildInfo`; no version, revision and time reported, `Modified` from `vcs.modified` |
| 4 | none of the above | `SourceUnknown`; every field empty |

**Validation rules**:

- `"(devel)"` MUST NOT be surfaced as a version. It is the module system's placeholder, and
  reporting it to a user is the same defect as reporting `"dev"` — FR-004 requires an explicit
  "unavailable" instead.
- `Version` is the single value the update path consumes (FR-005); the version command and the
  updater MUST NOT resolve it independently.
- `Source == SourceUnknown` is what triggers FR-017: the update command refuses to infer staleness
  and requires an explicit target version.

**Rendering** (FR-002): the version command prints version, commit, build time, and — when
`Modified` — an explicit dirty marker, alongside the existing Go version and os/arch lines.

---

## 2. Install provenance

How the running binary got where it is. Decides whether sting may replace it, and if not, what to
tell the user. Resolved by `internal/selfupdate` per invocation; never cached, never written down.

```go
type Owner int
const (
    OwnerUnmanaged Owner = iota // hand-placed — sting may replace it
    OwnerHomebrew
    OwnerRPM
    OwnerDPKG
    OwnerGoToolchain
    OwnerUndeterminable         // executable path could not be resolved
)

type Provenance struct {
    Owner          Owner
    RealPath       string // after EvalSymlinks
    UpgradeCommand string // the command to print when Owner != OwnerUnmanaged
}
```

**Classification order** — first match wins (research §5). Order matters because the checks
overlap; a Homebrew binary on `PATH` is a symlink into a versioned cellar.

```text
os.Executable()
  └─ filepath.EvalSymlinks          ← MUST happen first (spec symlink edge case)
       ├─ error                     → OwnerUndeterminable → refuse to write (FR-018)
       ├─ under brew prefix / Cellar / Caskroom → OwnerHomebrew
       ├─ rpm -qf <path> exits 0                → OwnerRPM
       ├─ dpkg -S <path> exits 0                → OwnerDPKG
       ├─ under $GOBIN | $GOPATH/bin | ~/go/bin → OwnerGoToolchain
       └─ otherwise                             → OwnerUnmanaged
```

**Validation rules**:

- Any `Owner` other than `OwnerUnmanaged` MUST produce a non-empty `UpgradeCommand`, exit non-zero,
  and write nothing (FR-008).
- `OwnerUndeterminable` MUST refuse rather than guess a path (spec edge case).
- Absence of `rpm`/`dpkg` means "not owned by that manager", never an error — those binaries only
  exist where their databases do (research §5).
- Homebrew detection is a path test, not an invocation of `brew`, so it holds when `brew` is not on
  `PATH`.

**Upgrade commands** by owner: `brew upgrade --cask sting`; `sudo dnf upgrade sting`; download the
release `.deb` then `sudo dpkg -i` (there is no apt repo — FR-025);
`go install github.com/skaphos/sting/cmd/sting@latest`.

---

## 3. Update plan and outcome

The resolved intent of one `sting update` invocation. Exists so the dry-run mode (FR-014) and the
real run share one decision path — dry-run computes a `Plan` and stops; a real run executes it.

```go
type Plan struct {
    Current    Info        // from buildinfo; may be SourceUnknown
    Target     string      // resolved tag, e.g. "v0.8.0"
    Provenance Provenance
    AssetName  string      // platform archive for this GOOS/GOARCH
    Action     Action
    Reason     string      // why, when Action is not ActionReplace
}

type Action int
const (
    ActionReplace Action = iota // proceed
    ActionUpToDate              // already current  (User Story 2, scenario 4)
    ActionDeferToManager        // package-manager owned              (FR-008)
    ActionGated                 // platform gate (Windows unsigned)   (FR-006)
    ActionRefuse                // unknown version, no asset, etc.
)
```

**Validation rules**:

- Draft and pre-release tags are excluded from "latest" resolution and reachable only by explicit
  name (FR-020).
- A tag whose assets or checksum manifest are not yet published resolves to "not available", not a
  verification failure (spec edge case).
- `AssetName` with no match for the running platform → `ActionRefuse`, naming the platform and the
  assets that do exist (spec edge case).
- On Windows, `Action` is forced to `ActionGated` while release binaries are unsigned (FR-006), and
  the reason names the gate so it reads as deliberate.

The exit-code mapping for each `Action` is the command contract — see
[contracts/cli-update.md](./contracts/cli-update.md).

---

## 4. Verification material

What must check out before a single byte is written. Not a persisted entity — a transient bundle of
downloaded material and the policy applied to it.

```go
type Material struct {
    Checksums       []byte // checksums.txt
    Bundle          []byte // checksums.txt.sigstore.json
    Artifact        []byte // the platform archive
    ExpectedIssuer  string // https://token.actions.githubusercontent.com
    ExpectedSANPat  string // https://github.com/skaphos/sting/.github/workflows/release.yml@refs/tags/v*
}
```

**Verification order** — each step gates the next (FR-009, FR-010):

1. Verify the Sigstore bundle over `checksums.txt`, with **both** an artifact-digest policy and a
   certificate-identity policy.
2. Assert the certificate SAN matches the pinned release-workflow pattern and the issuer matches
   the pinned OIDC issuer.
3. Parse the now-trusted manifest; find the digest for `AssetName`.
4. Hash the downloaded archive; compare to that digest.

**Validation rules**:

- Step 2 is not optional and not inferable from step 1. A bundle signed by *any* Sigstore identity
  passes step 1 — the identity pin is the entire control (FR-010).
- No flag, environment variable, or config key may skip any step (FR-012).
- Any failure leaves the installed binary untouched and names the failing step (FR-011).
- Verification needs network access for the trust root; when it cannot complete, the update
  refuses rather than degrading to checksum-only (spec edge case).
- A valid signature from a non-matching identity is a *required negative test*, not a hypothetical
  (spec edge case, SC-013).

---

## 5. Server description (`server.json`)

The checked-in declaration of sting's MCP identity. The only entity here that lives in the
repository rather than in memory. Full field-by-field contract in
[contracts/server-json.md](./contracts/server-json.md).

**Validation rules**:

- `name` MUST be `io.skaphos/sting` — the reverse-DNS form of the domain proving ownership
  (FR-026).
- `version` MUST equal the release tag at publish time (FR-028); drift is what post-release
  verification catches.
- Schema-validated in CI on every change (FR-027), so an invalid entry fails before a release
  rather than at publish time.
- Updated in the same change as any change to advertised capabilities (FR-030) — sting currently
  advertises two read-only tools, `get_commits` and `get_repo_activity`.

---

## 6. Release artifact set

Everything one release publishes, plus which of them actually landed. Not a Go type — it is the
assertion surface of the post-release verification job. Enumerated in
[contracts/release-artifacts.md](./contracts/release-artifacts.md).

**Validation rules**:

- Archives and Linux packages appear in the signed checksum manifest; the container image carries
  its own signature and attestations instead (SC-009 — the image is *not* in the manifest).
- Verification asserts the **version each channel serves**, not merely that it responds: a cask
  still pinned to the previous release responds perfectly well and is the exact failure being
  guarded against (FR-039, spec edge case).
- Every channel except the MCP registry blocks the release on failure (FR-040); the registry is
  reported and non-blocking (FR-029).
- A missing or unusable credential fails the release rather than silently skipping its channel
  (FR-038).

---

## Testability seams

Recorded here because the plan's Constitution Check treats them as a design constraint, not an
implementation detail: they are what make the 80% coverage floor reachable for `internal/selfupdate`
without touching the network.

| Seam | Injected for tests as | Enables testing |
| --- | --- | --- |
| HTTP client | `httptest` server | release resolution, asset download, rate-limit and partial-publish paths |
| Executable path resolver | function returning a `t.TempDir()` path | every `Provenance` branch, including symlink resolution |
| Command runner (`rpm`, `dpkg`) | stub returning a canned exit status | `OwnerRPM` / `OwnerDPKG` without those tools installed |
| Filesystem root | `t.TempDir()` with isolated `HOME`/`USERPROFILE` | atomic replace, permission failure, interrupted replace |
| Trust root / verifier | fixture bundles, including a valid-signature-wrong-identity case | SC-013's required negative test |
