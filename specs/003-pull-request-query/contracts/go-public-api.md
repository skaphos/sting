# Contract: Public Go PR API

The declarations below are implemented. They reuse existing package import paths and `ghclient.New`;
no consumer dependency or public transport framework is introduced.

## `config`

```go
func (cfg Config) ResolvePRs(req PRRequest, now time.Time) (model.PRQuery, error)
func (cfg Config) ResolvePRInbox(req PRInboxRequest) (model.PRInboxQuery, error)
```

`PRRequest`: raw Provider, Scope, Author, Repos, Org, Since, Until, Window, State, TimeBasis,
Role, `Draft *bool`, `MaxPRs *int`, `MaxRequests *int`.
`PRInboxRequest`: raw Provider, Scope, User, Repos, Org, `Draft *bool`, `MaxPRs *int`,
`MaxRequests *int`; intentionally no activity fields.

Add `Config.MaxPRs`, `DefaultMaxPRs=100`, and the matching `Default()`/`Defaults()`/validation
entries. No state/role/draft defaults are read from unrelated commit flags. Factor small private
helpers for applicable PR defaults, targets, limits, and login validation; leave existing
`Resolve`/`ResolveActivity` behavior intact. Inbox user validation is local; authenticated identity
resolution is the collector's read-only boundary and does not introduce I/O into `config`.

## `ghclient`

```go
func (c *Client) CollectPRs(ctx context.Context, q model.PRQuery) (model.PRResult, error)
func (c *Client) CollectPRInbox(ctx context.Context, q model.PRInboxQuery) (model.PRInboxResult, error)
```

Require normalized queries from `config`; defensively reject invalid provider/targets/caps at
entry before any request. A query's MaxRequests is enforced even when `New` was called without
`WithRequestBudget`. New collectors create private per-call sessions from the client's immutable
construction settings; they do not reset/mutate existing commit/activity counters or share an
exhausted transport. Keep token/base URL settings private and never serialize/debug-print them.

Public clients may be reused for successive or concurrent PR calls; budgets are independent.
A private session can be built using the existing constructor with its own budget option, then
an unexported collection method executes on that session (avoid recursive public dispatch).
Existing `New` calls/options stay source-compatible; preserve original transport behavior for
existing collection methods.

Return initialized typed reports with attributable disclosures on collection failure, including
zero-match failures. Caller branches with `errors.Is/As` for wrapped errors. Own cap stops
return a nil error; provider rate limiting remains an error. No package-level mutable test hook
is necessary in collectors; existing base-URL injection supports local HTTP fixtures.

## `model`

Add the two independent schema constants and the query, PR metadata, match/relationship,
entry, coverage, cost, disclosure, and result types in [data-model.md](../data-model.md).
Do not change `SchemaVersion` or `ActivitySchemaVersion`. A future breaking change to shared
PR metadata requires bumping both PR envelopes if both are affected.

The JSON result mirrors public types; Markdown may not contain extra facts. Required evidence
collections are initialized empty slices; optional metadata distinguishes unavailable from known
empty/zero. Do not expose raw go-github payloads or internal transport types through the API.

## Internal adapters

Add a narrow `PRClient` interface in `internal/commitclient` with both collection methods and a
`NewPR(cfg)` GitHub constructor that reuses `resolveGitHubToken`, configured base URL and per-page.
The public collectors enforce query budgets themselves. Keep existing factories unchanged.
CLI/MCP inject this dependency at their boundary for adapter tests, following current patterns.

Add `render.RenderPRs` / `render.PRsMarkdown` and `render.RenderPRInbox` /
`render.PRInboxMarkdown`. Reuse existing format parsing and small shared PR metadata rendering
helpers; avoid a result union with runtime mode switches.

## Compatibility checks

External-package tests import `model`, `config`, and `ghclient`, resolve and collect both
workflows against httptest, and verify a small budget without passing a constructor budget
option. Add successive/concurrent-call isolation tests. Existing import examples and commit/
activity tests remain valid. No `go.mod`/`go.sum` changes are planned.
