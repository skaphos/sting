# Contract: Credential Report Line and Verbosity Setting

## Verbosity setting

| Surface | Form |
|---------|------|
| Flag (persistent, every command) | `--verbose` |
| Environment | `STING_VERBOSE=true` |
| Config file | `verbose: true` |

Precedence follows sting's standard order: flag > env > file > default (`false`). With the
setting off, sting's output on every stream is unchanged (FR-003).

## Report line

Exactly one line per credential resolution, written to **stderr** and never to stdout (FR-005):

```text
sting: credential: <provider> <host>: <source>
```

- `<provider>`: `github` | `gitlab`
- `<host>`: bare hostname, e.g. `github.com`, `gitlab.example.com`
- `<source>`: one of

```text
dedicated PAT
stored oauth token (keyring)
stored pat (keyring)
stored oauth token (file)
stored pat (file)
ambient token from environment (opt-in)
ambient token from gh config (opt-in)
anonymous
anonymous (stored-credential lookup failed: <reason>)
```

`<reason>` is one of the following:

- the credential store's own `Load` error, which names only the provider, the host, and the
  keyring/OS error (for example, a locked keyring); or
- the fixed text `credentials file could not be read`, used when the store cannot be opened.
  The underlying error is deliberately dropped, because a YAML parse error can quote the
  offending line of `hosts.yml`, and that line may hold a token.

Neither form ever contains token material (FR-009).

### Examples

```text
sting: credential: github github.com: dedicated PAT
sting: credential: gitlab gitlab.example.com: stored oauth token (file)
sting: credential: github github.com: anonymous (stored-credential lookup failed: a stored credential for github/github.com exists but the keyring is unavailable: …)
```

## Stability

The line is a human diagnostic, not a machine contract. It is not covered by
`model.SchemaVersion`, and it may be reworded in a minor release. Tooling must not parse it.
The structured result is the evidence contract (Principle II).
