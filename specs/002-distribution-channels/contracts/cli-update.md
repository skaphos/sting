# Contract: `sting update` — NOT SHIPPED

**Feature**: [../spec.md](../spec.md) | **Date**: 2026-07-25

> **This contract describes a command that does not exist.** It was specified, implemented
> against this contract, and then dropped: verifying releases in-process cost +122% binary size
> and 62 extra modules to serve one command. See
> [ADR 0011](../../../docs/adr/0011-no-self-update-subcommand.md).
>
> It is retained because it is the specification a future implementation would have to meet,
> and because ADR 0011's reasoning only makes sense against a concrete description of what was
> given up. The `sting version` section at the end **did** ship.

The ninth top-level command, alongside `query`, `activity`, `mcp`, `install`, `uninstall`,
`version`, `auth`, `init`. Thin Cobra wrapper over `internal/selfupdate`.

## Synopsis

```text
sting update [--check] [--version <tag>]
```

| Flag | Type | Default | Meaning |
| --- | --- | --- | --- |
| `--check` | bool | `false` | Report what would happen; download nothing, write nothing (FR-014) |
| `--version` | string | latest stable | Install this exact tag; enables rollback (FR-016) |

**There is no flag to skip verification.** FR-012 forbids one, and none may be added — not behind
an environment variable, a config key, or a hidden flag.

## Exit codes

The contract callers and CI can rely on. One code per outcome class, so scripting against it is
possible without parsing prose.

| Code | Outcome | Wrote anything? |
| --- | --- | --- |
| `0` | Updated successfully, or already up to date | binary replaced / nothing |
| `1` | Verification failed — signature, identity, or checksum mismatch | **no** (FR-011) |
| `2` | Binary is owned by a package manager; upgrade command printed | **no** (FR-008) |
| `3` | Platform gate: self-replacement disabled on Windows pending signing | **no** (FR-006) |
| `4` | Refused — version unknown, no asset for platform, tag not found, assets not yet published | **no** |
| `5` | Filesystem/permission failure during replacement | binary left intact (FR-013, FR-018) |

Non-zero on every outcome except success and up-to-date, per FR-008's "exit non-zero" and the
package-manager and gate scenarios in User Story 2.

## Output

Human-readable on stdout; failures name the reason and the next safe action (Principle II).

**Successful update** (FR-015):

```text
sting v0.7.0 -> v0.8.0
  verified: publisher signature and checksum OK
  replaced: /usr/local/bin/sting
```

**Already current** (User Story 2, scenario 4) — exit `0`, no download:

```text
sting v0.8.0 is already the latest release.
```

**Dry run** (`--check`, FR-014) — reports the whole resolved plan, writes nothing:

```text
sting update --check
  current:   v0.7.0        (source: build metadata)
  latest:    v0.8.0
  install:   /usr/local/bin/sting (unmanaged)
  action:    would replace
```

**Package-manager owned** (FR-008) — exit `2`:

```text
sting was installed with Homebrew and must be upgraded through it:

    brew upgrade --cask sting

Refusing to overwrite a package-managed file.
```

The same shape for rpm (`sudo dnf upgrade sting`), dpkg (download the release `.deb`, then
`sudo dpkg -i` — there is no apt repository, FR-025), and the Go toolchain
(`go install github.com/skaphos/sting/cmd/sting@latest`).

**Verification failure** (FR-011) — exit `1`, names the failing step:

```text
Update refused: certificate identity mismatch.
  expected signer: https://github.com/skaphos/sting/.github/workflows/release.yml@refs/tags/*
  actual signer:   https://github.com/someone-else/fork/.github/workflows/release.yml@refs/tags/v1.0.0
The installed binary was not modified.
```

**Windows gate** (FR-006) — exit `3`, legible as deliberate rather than broken:

```text
sting v0.8.0 is available.

Self-update is not enabled on Windows: release binaries are not yet
Authenticode-signed, and sting will not ask you to self-install an
unsigned replacement.

Download: https://github.com/skaphos/sting/releases/tag/v0.8.0
```

**Unknown current version** (FR-017) — exit `4`:

```text
Update refused: this build records no version, so sting cannot tell whether
it is out of date. Re-run with an explicit target, e.g.:

    sting update --version v0.8.0
```

## Behavioral requirements

- **No implicit network calls** (FR-007). `update` is the only command that may contact a release
  or version endpoint. No other command performs an update check, on any schedule or trigger.
- **No credentials** (Principle IV, plan gate IV). Release assets are public; `update` sends no
  token and MUST NOT read `GITHUB_TOKEN`, `GH_TOKEN`, or sting's own `STING_TOKEN`. Unauthenticated
  fetches face stricter rate limits — that is a failure mode to report clearly (exit `4`), not a
  reason to authenticate.
- **No telemetry** (FR-019). Nothing identifying is transmitted beyond what fetching a public
  asset inherently requires.
- **Atomic replacement** (FR-013). Interruption leaves exactly one complete binary.
- **Draft and pre-release** tags are never selected as latest; reachable only via `--version`
  (FR-020).

## Changes to `sting version`

Same command, same output shape, one added line when the build is dirty (FR-002), and honest
values where it previously printed placeholders (FR-001, FR-004).

```text
sting v0.7.0
  commit:  9ab1f88... (modified)
  built:   2026-07-25T10:04:11Z
  go:      go1.26.5
  os/arch: linux/amd64
```

When nothing is recorded (FR-004) — an explicit unavailable, never `dev` and never `(devel)`:

```text
sting (version unavailable)
  commit:  unknown
  built:   unknown
  go:      go1.26.5
  os/arch: linux/amd64
```

Release builds are unchanged: ldflags win (FR-003), so what GoReleaser stamps is what prints.
