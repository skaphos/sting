# 2. Dedicated GitHub PAT via viper, separate from GITHUB_TOKEN

## Status

Accepted.

## Context

sting needs a GitHub token to raise rate limits and to read private repositories
(including SSO-authorized orgs). The obvious default is the ambient
`GITHUB_TOKEN` environment variable, but that variable is shared with many other
tools and is often a broad-scoped or short-lived token. sting only ever reads,
so it wants a narrow, long-lived, read-only PAT that does not collide with
whatever `GITHUB_TOKEN` happens to hold in a given shell.

## Decision

Resolve configuration with viper (`internal/cli`) in increasing precedence:
built-in defaults < config file < environment (`STING_*`) < flags. The token is
sting's own key:

- config file: `token: <pat>` (YAML, discovered under `$XDG_CONFIG_HOME/sting`,
  `~/.config/sting`, `~/.sting`, or `.`, or via `--config`)
- environment: `STING_TOKEN`
- flag: `--token`

`GITHUB_TOKEN` is intentionally **not** read. `internal/config.Config` carries
mapstructure-tagged keys; the CLI seeds viper defaults from
`config.Defaults()`, binds the config-bearing flags, and unmarshals into the
struct, then validates.

## Consequences

- A dedicated read-only PAT lives in sting's config and is unaffected by the
  ambient `GITHUB_TOKEN`.
- Configuration is uniform: every setting has a default, a config-file key, and
  an `STING_`-prefixed env override; the config-bearing ones also have flags.
- Users who *want* to reuse `GITHUB_TOKEN` must set `STING_TOKEN` (or the config
  key) explicitly — a deliberate, one-line opt-in rather than a silent default.

## Alternatives Considered

- **Read `GITHUB_TOKEN` as the default.** Rejected: couples sting's credential
  to an unrelated, often broad token; the whole point was a separate PAT.
- **Hand-rolled env/file loader** (the first iteration). Replaced by viper for
  precedence, discovery, and flag binding without bespoke code.
- **Fall back to `GITHUB_TOKEN` when `STING_TOKEN` is unset.** Rejected as
  surprising; explicit opt-in is clearer and matches the stated requirement.
