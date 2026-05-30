# 6. GitLab provider support

## Status

Accepted.

## Context

sting originally queried only GitHub. Its public packages were therefore shaped
around one provider: `ghclient` performed all remote access, `config` carried a
single `token` and `base_url`, and `model.Query` / `model.Result` did not need
to record the provider that produced the evidence.

GitLab support needs the same high-level behavior: retrieve a user's commits
over a time window, render them locally, or expose them through the read-only
`get_commits` MCP tool. GitLab's API does not match GitHub's commit search
surface exactly. Repository commits and group project enumeration map cleanly to
sting's `repos` and `org` scopes, but GitLab's search API does not provide the
same date-bounded author query contract as GitHub commit search.

## Decision

Add a first-class provider concept to the public domain model:

- `model.Provider` identifies `github` or `gitlab`.
- `model.Query` carries the requested provider.
- `model.Result` records the provider that produced the result.

Keep GitHub as the default provider. Existing CLI and MCP calls that omit a
provider continue to query GitHub with the existing `token` and `base_url`
settings.

Add GitLab-specific configuration keys instead of reusing GitHub's credential:

- `gitlab_token` / `STING_GITLAB_TOKEN`
- `gitlab_base_url` / `STING_GITLAB_BASE_URL`

Implement GitLab in a separate public `gitlabclient` package and keep `ghclient`
focused on GitHub. The internal application layer selects the correct provider
client through `internal/commitclient`, so CLI and MCP request handling share
the same dispatch behavior.

Support GitLab `repos` and `org` scopes first:

- `repos` queries `GET /projects/:id/repository/commits`.
- `org` treats `org` as a GitLab group, lists group projects with
  `include_subgroups=true`, then queries each project's commits.
- `include_stats` maps to GitLab's `with_stats` list option.

Do not implement GitLab `search` scope yet. Return a clear validation error for
`provider=gitlab` with `scope=search`, and document that limitation.

The `get_commits` MCP tool remains the only exposed tool and remains read-only.

## Consequences

- Users can query GitHub and GitLab with the same CLI/MCP request shape.
- GitHub behavior remains backward compatible because `github` is the default.
- GitLab credentials stay separate from GitHub credentials and ambient provider
  tokens.
- Consumers of `model.Result` get an additive `provider` field. This is not a
  breaking result-shape change, so `model.SchemaVersion` remains
  `sting.skaphos.io/v1`.
- GitLab global commit search is intentionally unavailable until its behavior
  can be mapped to a coherent Sting scope contract.

## Alternatives Considered

- **Reuse `ghclient` for GitLab.** Rejected: it would mix provider-specific API
  behavior and make the GitHub package name misleading.
- **Create separate MCP tools per provider.** Rejected: provider choice is a
  request parameter, and keeping one read-only tool preserves the installer and
  auto-approval safety model.
- **Reuse `token` / `base_url` for both providers.** Rejected: it would blur
  credential ownership and break the existing dedicated-GitHub-PAT invariant.
- **Implement GitLab search immediately.** Rejected for the first pass because
  GitLab search does not match the existing date-bounded author query semantics.
