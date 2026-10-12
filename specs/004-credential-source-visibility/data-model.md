# Data Model: Credential Source Visibility

A single internal record, `commitclient.Resolution`. It is diagnostic only: it is never part of
`model.Result`, and it never crosses the public package boundary (ADR 0004), so
`model.SchemaVersion` does not change.

## Resolution

| Field | Type | Meaning |
|-------|------|---------|
| `Provider` | `model.Provider` | `github` or `gitlab`. |
| `Host` | `string` | Bare hostname used as the credential-storage key (`credentialHost`), e.g. `github.com`, `ghe.example.com`. |
| `Source` | `Source` (internal enum) | The selected tier; see below. |
| `Kind` | `credentials.TokenType` | `oauth` or `pat`. Set only for stored and ambient sources. |
| `LookupErr` | `error` | Non-nil when the stored-credential lookup failed for a reason other than `credentials.ErrNotFound`. |

The token value is **not** a field. A `Resolution` cannot carry a secret, by construction
(FR-009).

### Source values

| Value | Selected when | Report label |
|-------|---------------|--------------|
| `dedicated` | `cfg.Token` / `cfg.GitLabToken` is non-empty | `dedicated PAT` |
| `keyring` | store `Load` returns `credentials.SourceKeyring` | `stored <kind> (keyring)` |
| `file` | store `Load` returns `credentials.SourceFile` | `stored <kind> (file)` |
| `ambient-env` | store returns `credentials.SourceEnv` (only possible under `STING_ALLOW_AMBIENT_GITHUB_TOKEN`) | `ambient token from environment (opt-in)` |
| `ambient-gh` | store returns `credentials.SourceConfig` (same opt-in) | `ambient token from gh config (opt-in)` |
| `anonymous` | none of the above | `anonymous` |

### Rules

- Precedence is unchanged: `dedicated` beats the store, which beats `anonymous` (FR-010).
- `LookupErr` is set only when the source is `anonymous`. A failed lookup is the only way to
  reach anonymous through an error.
- Exactly one `Resolution` per constructor call, so one per CLI run and one per MCP tool call
  (FR-007).
