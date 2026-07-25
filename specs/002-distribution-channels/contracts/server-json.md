# Contract: `server.json` — MCP registry entry

**Feature**: [../spec.md](../spec.md) | **Date**: 2026-07-25

Checked in at the repository root. Declares sting's identity to the MCP registry (FR-026–FR-030).

## Identity

| Field | Value | Source |
| --- | --- | --- |
| Namespace | `io.skaphos` | reverse-DNS of `skaphos.io`, proven by DNS TXT (clarification Q2) |
| Name | `io.skaphos/sting` | FR-026 — the canonical identity MCP clients use |
| Version | the release tag, without the `v` | FR-028 — must equal the release at publish time |

The registry derives namespace authority from the authentication method: domain authentication
requires the name to be the reverse-DNS form of a domain the publisher controls. `io.skaphos/*` is
therefore only publishable by whoever holds the `skaphos.io` TXT proof.

## Shape

Schema: `https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json`

```jsonc
{
  "$schema": "https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json",
  "name": "io.skaphos/sting",
  "description": "Query a GitHub or GitLab user's commits over a time window. Read-only.",
  "repository": {
    "url": "https://github.com/skaphos/sting",
    "source": "github"
  },
  "version": "0.8.0",
  "packages": [
    {
      "registryType": "oci",
      "identifier": "ghcr.io/skaphos/sting",
      "version": "0.8.0",
      "transport": { "type": "stdio" },
      "environmentVariables": [
        {
          "name": "STING_TOKEN",
          "description": "GitHub personal access token with read-only repository scope.",
          "isRequired": false,
          "isSecret": true
        },
        {
          "name": "STING_GITLAB_TOKEN",
          "description": "GitLab personal access token with read-only scope.",
          "isRequired": false,
          "isSecret": true
        }
      ]
    }
  ]
}
```

## Field rules

- **`description`** MUST describe verified behavior and state the read-only scope (Principle VIII,
  Principle I). No marketing language.
- **`packages[].registryType`** is `oci` — the container image is the registry-discoverable
  artifact, which is why FR-031 (image) and FR-026 (entry) ship together.
- **`packages[].version`** tracks the image tag and therefore the release tag.
- **`transport.type`** is `stdio`: `sting mcp` serves over stdio and owns stdout, matching the
  image entrypoint (FR-032) and what `sting install` already registers for local runtimes.
- **`environmentVariables`** are sting's own `STING_`-prefixed keys, per Principle IV. Ambient
  provider tokens (`GITHUB_TOKEN`, `GITLAB_TOKEN`) MUST NOT appear here — sting does not read
  them, and advertising them would misrepresent the credential model.
- Both tokens are `isRequired: false`: sting starts without one and reports the missing credential
  when a query needs it, rather than refusing to boot.
- **`isSecret: true`** on both, so clients mask them.

## Capability drift

sting currently advertises two read-only tools, `get_commits` and `get_repo_activity`
(`internal/mcpserver/server.go`, ADR 0010). FR-030 requires this file to be updated in the same
change as any change to advertised capabilities, so the registry never describes a surface sting
does not have.

## Validation

- **CI** validates this file against the published schema on every change (FR-027), so an invalid
  entry fails in the pull request rather than at publish time in a release.
- **Release** publishes it after the image exists, since the entry names an image tag that must
  already resolve.
- **Post-release verification** queries the registry for `io.skaphos/sting` and asserts the version
  equals the tag — reported but non-blocking (FR-029, FR-040).

## Publishing

Non-interactive, from the release workflow:

```sh
mcp-publisher login dns --domain skaphos.io --private-key "$MCP_REGISTRY_KEY"
mcp-publisher publish
```

- `MCP_REGISTRY_KEY` is an Ed25519 private key held as an **organization-scoped** secret, not an
  individual's (FR-028).
- The corresponding TXT record at the `skaphos.io` apex:
  `v=MCPv1; k=ed25519; p=<base64 public key>`.
- Google KMS and Azure Key Vault are supported alternatives for the same proof if key custody
  becomes a concern; neither changes the namespace or the TXT record.

**Failure handling** (FR-029): a publish failure is surfaced in the release run's output, does not
invalidate the rest of the release, and is retryable without cutting a new release. The registry is
in preview with breaking changes and data resets possible, which is the documented basis for it
being the one channel exempt from failing the release as a unit.
