# Quickstart Validation: PR Activity and Personal Inbox

This guide validates the implemented PR activity and personal inbox workflows. Run commands
from the repository root; a missing fixture test must not be mistaken for a passing feature.
Recorded implementation checkpoints and final gates appear below.

## Prerequisites

- Go version pinned in `go.mod`, with dependencies available locally.
- Standard repository task runner via `go -C tools tool task`.
- No real GitHub token or network is needed for automated validation. Fixtures must isolate
  HOME/USERPROFILE and all STING/provider environment settings and use local `httptest` servers.

## Build and verify the executable path

```sh
go -C tools tool task build
./sting prs --help
./sting inbox --help
go test -list '^TestPRCLIAndMCP$' ./integration | rg '^TestPRCLIAndMCP$'
go test -race -count=1 ./integration -run '^TestPRCLIAndMCP$'
```

The implementation adds `TestPRCLIAndMCP` alongside the existing integration suite. It builds
the actual executable, starts a local provider fixture, and exercises Cobra/config/provider/
rendering plus MCP initialization, tool listing and tool calls. It does not require an MCP host
application. Tests inject the local API base URL and compare normalized outputs excluding
`generated_at` and any measured wall-clock data.

## Required fixture scenarios

| Scenario | Expected observable result |
| --- | --- |
| Author activity: opened draft, merge, unmerged closure in fixed window | All three action kinds included, unrelated author and stale-open-only PR excluded |
| Old PR closed in window then reopened later | Historical closure appears even though current state is open |
| Repeated close/reopen cycles, merge-associated closure | One PR, distinct confirmed closures, one merge; ambiguous closure disclosed instead of counted twice |
| Search candidate ceiling with few actual matches | `search-capped`; incomplete discovery despite small/empty matching output |
| Paginated history stops at request budget | Confirmed matches retained; history incomplete; own-cap exit 0 / MCP success |
| Provider error after partial collection, including zero matching PRs | Versioned JSON remains on stdout / structured MCP output; exit 1 / `IsError=true` |
| Old authored, assigned, direct-review-requested PRs | Inbox includes all three with reasons; does not inherit activity window |
| Team-only review request, fulfilled request, removed assignment | No personal match without another supported relationship |
| One PR matching every relationship | One entry with three ordered reasons; counts once |
| Search-inbox list verification stops early | Confirmed authored/assigned entries retained; unverified review-only candidates excluded; reason coverage disclosed |
| Search assignment contradicted by a known empty assignment list; another PR still authored | Invalid assigned reason removed; unrelated PR excluded; authored PR retained with only its valid reasons; `provider-changed` and incomplete evidence |
| Candidate absent from complete open-PR listing, then repeated by another search stream | Candidate removed and not re-admitted; absence reason/state not invented; zero-entry result still retains costs and coverage gap |
| Listing interrupted before candidate, versus contradiction observed before a later page fails | Absence alone retains uncontradicted proof; already observed contradiction still removes invalid proof; both disclose incomplete coverage |
| MCP starts with invalid unused `default_window` | Initialization and four-tool listing succeed; inbox succeeds; activity relying on invalid default and legacy calls fail locally with zero provider requests; a subsequent inbox call succeeds on the same server |
| Malformed/type-invalid config or invalid shared startup settings | MCP startup fails clearly; no provider requests; standalone commands retain their existing validation |
| Private visible repo plus unreadable org repo | Accessible evidence plus specific skip; unrestricted search uses public-only selector |
| Missing optional metadata vs missing inclusion proof | Null/empty distinctions; metadata gap alone does not truncate membership; unconfirmed candidates never match |
| Concurrent tools and reused public client | Each query independently honors its cap; no state/credential/default leakage |

Fixture request recording must reject non-GET methods and any detail/diff/review/check endpoint
outside [the collection allowlist](contracts/provider-collection.md). Verify that consumed equals
actual dispatched requests and the identity/discovery/history categories sum correctly. Ensure
read-only tools/list contains get_commits, get_repo_activity, get_prs, and get_pr_inbox.

## Package and repository gates

```sh
go test -race ./model ./config ./ghclient ./internal/commitclient ./internal/cli ./internal/mcpserver ./internal/render
go -C tools tool task test-integration
go -C tools tool task test-cover-check
go -C tools tool task ci
```

Expected: all tests pass; package coverage floors remain unchanged; existing commit/activity
fixtures still pass; no live provider calls are made. Full CI/release checks require the pinned
local tooling documented in AGENTS.md. See the recorded implementation checkpoints for actual commands and results.

## Optional human smoke check after implementation

Use an already configured dedicated sting credential and a repository you can read. The commands
below are read-only but do consume provider requests; substitute real targets and dates.

```sh
./sting prs --scope repos --repos OWNER/REPO --since 2026-08-24 --until 2026-08-25 --max-requests 20 -o json
./sting inbox --scope repos --repos OWNER/REPO --max-requests 20 -o json
```

The activity result records UTC bounds and qualifying actions; the inbox records the resolved
user and reasons without a time window. Small caps may produce an explicitly partial result.
Compare JSON with Markdown using [CLI semantics](contracts/cli.md), not assumptions about live
repository history. Do not embed tokens in examples, fixtures, logs, or shell history.

## Implementation checkpoints

- T001 baseline: `go -C tools tool task test` passed all packages on 2026-09-05.
  Dependencies remain pinned; the existing graph report is stale relative to HEAD.
- T003–T013 foundation: `go test ./model ./config ./ghclient ./internal/cli ./internal/mcpserver` passed. Local HTTP fixtures require execution outside the sandbox; no live provider calls were made.
- T014–T034 activity/search/repos checkpoints: `go test ./ghclient ./internal/commitclient ./internal/render ./internal/cli ./internal/mcpserver ./integration` passed after fixing the direct Cobra test context and registry expectation.
- T035–T040 organization checkpoint: `go test ./ghclient ./internal/cli ./internal/mcpserver ./integration` passed, covering skippable access failures and fatal authentication/rate-limit/provider errors.
- T041–T054 inbox checkpoint: `go test ./...` passed with four tools; external-package examples and public budget isolation passed under `go test ./ghclient -run 'TestPR|ExampleClient_CollectPR'`.
- T059 executable fixture: `go test -race -count=1 ./integration -run '^TestPRCLIAndMCP$'` passed for all scopes, bounded/error output, reconciliation, schema rejection, and startup isolation.

### Final validation (2026-09-05)

- `go -C tools tool task build` — passed; `./sting prs --help` and
  `./sting inbox --help` expose the documented flags and separate workflow semantics.
- `go test -race -count=1 ./integration -run '^TestPRCLIAndMCP$'` — passed; tests
  exercised the actual binary and stdio protocol against local provider fixtures.
- `go -C tools tool task test-race` — passed across all packages.
- `go -C tools tool task test-cover-check` — passed every package floor. Relevant coverage:
  config 94.24%, ghclient 88.43%, CLI 70.58% (60% floor), MCP 89.55%, render 92.75%, model 100%.
- `go -C tools tool task ci` — passed: zero lint issues, staticcheck passed, no vulnerabilities
  found, tests/integration passed, and snapshot builds succeeded for Linux/macOS/Windows on
  amd64 and arm64. Initial lint failures were corrected (deprecated pointer helpers, test
  constants/switch style, and import formatting); no checks were disabled.
- The request recorder rejects unexpected endpoints and non-GET methods. The pagination guard
  test proves an explicitly uncapped request still terminates at the disclosed safety bound.
- `go.mod` and `go.sum` are unchanged; license notice regeneration was not required.
- No live-provider smoke queries, commits, pushes, or remote mutations were performed.

Final-review regressions also verify that malformed API URLs do not expose embedded credentials,
and that complete history can settle a known closed record without `closed_at`. An unavailable
current state remains unknown rather than being guessed from a historical closure timestamp.
The required gates were rerun after these fixes.
