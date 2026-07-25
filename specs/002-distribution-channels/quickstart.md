# Quickstart: Validating Distribution Channel Conformance

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-07-25

Runnable validation for each user story. Local checks need no release; the channel checks run
against a snapshot build or a real tag, as noted.

## Prerequisites

```sh
go version                      # 1.26.5+
go -C tools tool task --list    # repo task runner
docker buildx version           # image validation
podman --version || true        # optional: package install checks without root
```

Full local gate before pushing, unchanged by this feature:

```sh
go -C tools tool task ci        # lint, staticcheck, vuln, race tests, coverage gate
```

---

## US1 — Version identity (P1)

**Validates**: FR-001–FR-005, SC-001. No release required.

```sh
# 1. Build with no release-time stamping — the `go install` path.
go build -o /tmp/sting-plain ./cmd/sting
/tmp/sting-plain version
```

Expect a real revision and a dirty marker if the tree is modified — never `dev`, never `(devel)`.

```sh
# 2. Install from the module proxy at a published version.
GOBIN=/tmp/gobin go install github.com/skaphos/sting/cmd/sting@latest
/tmp/gobin/sting version
```

Expect the resolved module version (e.g. `v0.7.0`). Revision is absent on this path — that is
correct, not a defect: module-proxy builds record no `vcs.*` settings (research §1).

```sh
# 3. Release-stamped build — ldflags must still win (FR-003).
go build -ldflags "-X github.com/skaphos/sting/internal/cli.Version=v9.9.9" \
  -o /tmp/sting-stamped ./cmd/sting
/tmp/sting-stamped version | head -1        # -> sting v9.9.9
```

**Pass**: every path reports honestly; the placeholder appears in none of them.

---

## US2 — `sting update` (P2)

**Validates**: FR-006–FR-020, SC-003–SC-006, SC-011–SC-014. Contract:
[contracts/cli-update.md](./contracts/cli-update.md).

Unit coverage carries most of this — the seams in
[data-model.md](./data-model.md#testability-seams) exist so none of it needs the network:

```sh
go test ./internal/selfupdate/... -race -v
```

Required cases, each mapping to a spec scenario:

| Case | Expect | Exit |
| --- | --- | --- |
| Tampered archive (checksum mismatch) | binary untouched, failing step named | 1 |
| **Valid signature, wrong identity** | rejected — the pin is the whole control (SC-013) | 1 |
| Binary under a simulated Homebrew prefix | prints `brew upgrade --cask sting`, writes nothing | 2 |
| Binary under a stubbed `rpm -qf` hit | prints the dnf command, writes nothing | 2 |
| Binary under `$GOBIN` | prints the `go install` command | 2 |
| Already latest | no download | 0 |
| Unknown current version | refuses, demands `--version` | 4 |
| No asset for platform | names platform and available assets | 4 |
| Unwritable target directory | names path and permission, no escalation | 5 |
| Interrupted mid-replace | exactly one complete binary | — |

Manual smoke once a release exists:

```sh
sting update --check          # resolves plan, writes nothing
sting update --version v0.7.0 # rollback path, same verification
```

**Verify no implicit network calls** (FR-007, SC-005) — the whole command surface must be silent:

```sh
go test ./internal/cli/... -run TestNoImplicitUpdateChecks -v
```

**Windows** (FR-006, SC-014): the gated path returns exit `3`, reports the available version, and
writes nothing. Self-replacement stays disabled until Authenticode signing lands.

---

## US3 — Linux packages (P3)

**Validates**: FR-021–FR-025, SC-002. Build without publishing:

```sh
goreleaser release --snapshot --clean
ls dist/*.deb dist/*.rpm      # expect 2 each: amd64, arm64
```

Install each on a matching image:

```sh
docker run --rm -v "$PWD/dist:/d:ro" debian:stable sh -c '
  dpkg -i /d/sting_*_amd64.deb && sting version && ls /usr/share/doc/sting/ && dpkg -r sting'

docker run --rm -v "$PWD/dist:/d:ro" fedora:latest sh -c '
  rpm -i /d/sting-*.x86_64.rpm && sting version && rpm -qf $(command -v sting) && rpm -e sting'
```

**Use `debian:stable`, not `debian:stable-slim`.** The slim image ships
`path-exclude /usr/share/doc/*` in its dpkg configuration, so the license and notice files are
silently dropped on install and the package looks broken when it is not. This is a property of
that image, not of the package.

**Pass**: binary on `PATH`, reports the snapshot version, `LICENSE` /
`THIRD_PARTY_NOTICES.md` / `third_party_licenses/` present under `/usr/share/doc/sting/`, the
package database claims the binary, and removal leaves nothing behind.

The `rpm -qf` check is doing double duty: it is also what `sting update` relies on to detect an
RPM-owned install and refuse to overwrite it.

```sh
grep -c '\.deb\|\.rpm' dist/checksums.txt    # packages are in the manifest (FR-024)
```

---

## US4 — MCP registry entry (P4)

**Validates**: FR-026–FR-030. Contract: [contracts/server-json.md](./contracts/server-json.md).

```sh
# Schema validation — the same check CI runs (FR-027).
check-jsonschema --schemafile \
  https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json server.json

jq -r '.name, .version' server.json          # -> io.skaphos/sting, matching the release
```

Capability drift check (FR-030) — the entry must not describe tools sting does not serve:

```sh
grep -o 'Name: "[a-z_]*"' internal/mcpserver/server.go   # get_commits, get_repo_activity
```

Publishing is exercised in the release workflow, not locally: it needs the org-scoped
`MCP_REGISTRY_KEY` and the `skaphos.io` TXT proof.

---

## US5 — Container image (P5)

**Validates**: FR-031–FR-035, SC-008.

```sh
goreleaser release --snapshot --clean
docker images | grep sting        # snapshot builds per-platform images, suffixed -amd64 / -arm64
```

Snapshot mode does **not** push, and therefore does not produce a multi-arch manifest — it builds
separate platform-suffixed images locally. The manifest assertion belongs to post-release
verification against a real tag:

```sh
docker buildx imagetools inspect ghcr.io/skaphos/sting:<released-version>
```

**Pass**: locally, both platform-suffixed images build; against a published tag, the manifest lists
`linux/amd64` and `linux/arm64` (FR-031).

Drive it as a real MCP server over stdio:

```sh
docker run --rm -i -e STING_TOKEN="$READONLY_PAT" ghcr.io/skaphos/sting:<tag>
```

Expect the server to start with no arguments (FR-032). With no token, expect a clear
credential-required message rather than a silent hang.

```sh
docker run --rm --entrypoint sh ghcr.io/skaphos/sting:<tag> -c 'id -u'   # non-root (FR-033)
```

---

## Cross-cutting — release coherence

**Validates**: FR-036–FR-041, SC-009, SC-010. Contract:
[contracts/release-artifacts.md](./contracts/release-artifacts.md).

```sh
goreleaser release --snapshot --clean
ls dist/          # 6 archives, 4 packages, checksums, bundle, SBOMs per archive AND package
```

The two behavioral properties that matter most, neither observable from a green release alone:

1. **A missing credential fails the release** (FR-038) rather than skipping its channel. Regression
   target: `release.yml`'s current `--skip=homebrew` path, which today drops the cask with only a
   `::warning::`.
2. **Verification asserts the version each channel serves** (FR-039), not that it responds. A cask
   pinned to the previous release responds fine and is exactly the failure being caught.

Post-release verification runs as its own job against a real tag; every channel blocks the workflow
on mismatch except the MCP registry, which warns (FR-029, FR-040).

---

## Traceability

| Story | Requirements | Success criteria |
| --- | --- | --- |
| US1 | FR-001–FR-005 | SC-001 |
| US2 | FR-006–FR-020 | SC-003–SC-006, SC-011–SC-014 |
| US3 | FR-021–FR-025 | SC-002 |
| US4 | FR-026–FR-030 | SC-007 |
| US5 | FR-031–FR-036 | SC-008 |
| Cross-cutting | FR-037–FR-043 | SC-009, SC-010 |
