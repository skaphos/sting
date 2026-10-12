# Data Model: Credential Source Visibility

A single unexported record, `resolution`, in `internal/commitclient/report.go`. It is
diagnostic only: it is never part of `model.Result`, and it never crosses the public package
boundary (ADR 0004), so `model.SchemaVersion` does not change.

## resolution

| Field | Type | Meaning |
|-------|------|---------|
| `provider` | `model.Provider` | `github` or `gitlab`. |
| `host` | `string` | Bare hostname used as the credential-storage key (`credentialHost`), e.g. `github.com`, `ghe.example.com`. |
| `source` | `credSource` | The selected tier; see below. |
| `kind` | `credentials.TokenType` | `oauth` or `pat`. Set only for stored and ambient sources. |
| `lookupReason` | `string` | Non-empty when a stored-credential lookup failed for a reason other than `credentials.ErrNotFound`. It never carries credential-file contents. |

The token value is **not** a field. A `resolution` cannot carry a secret, by construction
(FR-009).

### credSource values

| Value | Selected when | Report label |
|-------|---------------|--------------|
| `dedicated` | `cfg.Token` / `cfg.GitLabToken` is non-empty | `dedicated PAT` |
| `keyring` | store `Load` returns `credentials.SourceKeyring` | `stored <kind> (keyring)` |
| `file` | store `Load` returns `credentials.SourceFile` | `stored <kind> (file)` |
| `ambient-env` | store returns `credentials.SourceEnv` (only under `STING_ALLOW_AMBIENT_GITHUB_TOKEN`) | `ambient token from environment (opt-in)` |
| `ambient-gh` | store returns `credentials.SourceConfig` (same opt-in) | `ambient token from gh config (opt-in)` |
| `anonymous` | none of the above | `anonymous`, or `anonymous (stored-credential lookup failed: <reason>)` |

### Rules

- Precedence is unchanged: `dedicated` beats the store, which beats `anonymous` (FR-010).
- `lookupReason` is set only when the source is `anonymous`. A failed lookup is the only way to
  reach anonymous through an error.
- Exactly one `resolution` per constructor call, so one per CLI run and one per MCP tool call
  (FR-007).
