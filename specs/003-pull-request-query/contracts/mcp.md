# Contract: PR MCP Tools

Add two entries to `internal/mcpserver.toolDefinitions()`, alongside the two existing tools.
Each has `ReadOnlyHint: true` and `OpenWorldHint: true`; `ReadOnlyTools()` remains derived.
No installer-maintained duplicate list. The schema/stdio/drift tests must expect four tools.

## `get_prs`

Purpose: return opened/merged/closed activity on PRs within a window, with bounded historical
recovery. It is neither open-branch commit discovery nor a present-day attention queue.

| Input | Type | Default / constraint |
| --- | --- | --- |
| `provider` | string | github; explicit gitlab fails before collection |
| `scope` | string | configured search/repos/org |
| `author` | string | required for search, optional for repos/org |
| `repos` | array of strings | validated owner/repo targets; applicable defaults |
| `org` | string | validated org; applicable default |
| `since`, `until`, `window` | strings | same precedence and date/window syntax as CLI |
| `state` | string | all; open/merged/closed/all |
| `time_basis` | string | any; created/updated/merged/closed/any |
| `role` | string | author when filtered; no other role supported |
| `draft` | nullable optional boolean | omitted/null unrestricted; true only drafts; false excludes drafts |
| `max_prs`, `max_requests` | optional integers | configured defaults; zero explicitly uncapped; negatives rejected |

Output: concrete `model.PRResult`, schema `sting.prs.skaphos.io/v1`.

Example tool arguments:

```json
{
  "scope": "org",
  "org": "acme",
  "author": "octocat",
  "since": "2026-08-24T13:00:00Z",
  "until": "2026-08-25T13:00:00Z",
  "time_basis": "any",
  "max_prs": 100,
  "max_requests": 500
}
```

## `get_pr_inbox`

Purpose: find currently open PRs authored by, assigned to, or directly requesting review from
one user, regardless of age. It only finds/summarizes work; actions happen outside sting.

| Input | Type | Default / constraint |
| --- | --- | --- |
| `provider` | string | github; explicit gitlab fails before collection |
| `scope`, `repos`, `org` | as above | applicable configured targets and visibility rules |
| `user` | string | authenticated sting credential owner; explicit login avoids lookup |
| `draft` | nullable optional boolean | omitted/null unrestricted; true only; false exclude |
| `max_prs`, `max_requests` | optional integers | same shared configuration and limits |

No author, role, state, date, or time-basis input. Reject additional properties; the input schema
and tool description direct activity callers to `get_prs`. All three relationships are enabled.
Output: concrete `model.PRInboxResult`, schema `sting.pr-inbox.skaphos.io/v1`.

Example tool arguments:

```json
{
  "scope": "repos",
  "repos": ["acme/api", "acme/web"],
  "user": "octocat",
  "draft": false,
  "max_requests": 50
}
```

## Startup and workflow validation

`sting mcp` decodes configuration once into a typed snapshot. Config-file syntax/type errors
and invalid shared settings such as `per_page` and `max_requests` still fail startup. Use a
dedicated MCP loader in `internal/cli/mcp.go`, backed by the common-settings validator in
`internal/mcpserver.New`, rather than the existing all-workflow `loadConfig`/`Config.Validate`
gate. Direct in-process server construction uses the same shared validation boundary.

Validate workflow-specific settings at the tool boundary, before any provider request:

- `get_pr_inbox` uses applicable inbox validation and ignores activity windows and the commit
  provider default. `get_prs` validates its resolved activity window and defaults to GitHub.
- Existing `get_commits` and `get_repo_activity` handlers retain the prior `Config.Validate`
  checks before their existing resolvers. Invalid legacy settings now produce a tool-level
  validation error rather than preventing unrelated tools from starting. Existing standalone
  CLI loaders and public resolvers retain their validation behavior.
- A rejected tool call makes zero provider requests and leaves the server usable. Never repair
  or overwrite the shared config snapshot to make one workflow pass.

Executable/stdio tests must start with an invalid unused `default_window`, initialize and list
all tools, successfully call the inbox, reject an activity call that relies on that invalid
default, and successfully call the inbox again. Check legacy tool validation, direct `New`
construction, and startup rejection of malformed config and invalid shared settings separately.

## Result transport and safety

Use typed `GetPRsInput` / `GetPRInboxInput` and `mcp.AddTool`. Pointer bool/int fields preserve
omission versus explicit false/zero. Resolve via the same `config` methods as CLI, with a
single captured clock for activity. Never reuse a request budget or mutable query state between
calls; test concurrent invocations independently.

- Success or own-cap stop: `IsError=false`, structured result plus a Markdown text view.
- Provider/identity failure after report initialization, even at zero PRs: `IsError=true`,
  structured result plus the same disclosed Markdown view; handler error remains nil so SDK
  serialization does not discard evidence.
- Local validation or client setup failure: return the actionable error, no fabricated report.
- Handler panic: recover as a generic tool-level internal error without exposing panic payloads,
  credentials, request structs, or provider bodies. Keep the server alive.

Schema tests verify output presence/null/empty distinctions and unknown input rejection rather
than assuming all SDK defaults. Stdio tests exercise initialize → tools/list → tools/call for
both tools and preservation of structured evidence on failure. Installer re-install picks up
the derived tool names while retaining its existing atomic, format-preserving ownership rules.
Existing commit/repository-activity query behavior and schema versions remain unchanged; only
workflow-specific MCP configuration errors move from process startup to the affected tool call.
