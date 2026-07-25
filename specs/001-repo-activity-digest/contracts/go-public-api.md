# Contract: Go Public API

**Feature**: [../spec.md](../spec.md) | **Data model**: [../data-model.md](../data-model.md)

The public packages are the contract (Constitution V, ADR 0004). This file states exactly what
this feature adds, and — more importantly — what it does not change.

## Compatibility guarantee

**Nothing existing changes shape.**

| Symbol | Change |
|---|---|
| `model.Result`, `model.Commit`, `model.File`, `model.Query`, `model.Scope` | **None** |
| `model.SchemaVersion` (`sting.skaphos.io/v2`) | **Not bumped** |
| `ghclient.Client.Collect` | Signature unchanged; failure behavior improved (see below) |
| `gitlabclient` | **None** |

Downstream consumers that pin `sting.skaphos.io/v2` — Wake's evidence adapter among them — compile
and behave identically after this feature lands. That is the entire reason the clarification chose
a sibling type over extending `Result`.

## Added: `model` package

```go
const ActivitySchemaVersion = "sting.activity.skaphos.io/v1"
const DefaultMaxRequests = 500

type ActivityQuery struct { /* see data-model.md */ }
type ActivityResult struct { /* see data-model.md */ }
type ActivityCommit struct { /* see data-model.md */ }
type Boundaries struct { /* ... */ }
type ChangeSet struct { /* ... */ }
type ChangedPath struct { /* ... */ }
type Correlation struct { /* ... */ }
type CostReport struct { /* ... */ }
type Disclosure struct { /* ... */ }

// Summary returns the first line of the commit message.
func (c ActivityCommit) Summary() string
```

`model` continues to import nothing internal (inward-only layering).

## Added: `config` package

```go
// ActivityRequest is the unresolved, stringly-typed request, mirroring the
// existing config.Request pattern so CLI and MCP share one validation path.
type ActivityRequest struct {
    Provider      string
    Repo          string
    Ref           string
    Since         string
    Until         string
    Window        string
    Author        string
    IncludeDiffs  *bool
    MaxDiffBytes  *int
    EnrichCommits *int
    MaxRequests   *int
    EstimateOnly  bool
}

// ResolveActivity validates and normalizes an ActivityRequest into an
// ActivityQuery, applying flags > env > file > defaults precedence. It is the
// single place the window is normalized (Principle III) and the single place a
// non-GitHub provider is rejected (FR-025).
func (cfg Config) ResolveActivity(req ActivityRequest, now time.Time) (model.ActivityQuery, error)
```

Added to `Config`: `MaxRequests int`, with the `max_requests` key in `Defaults()`.

### Error contract for `ResolveActivity`

| Condition | Message shape |
|---|---|
| Non-GitHub provider | `provider %q does not support repository activity (github only)` |
| Missing repo | `repo is required (owner/name)` |
| Malformed repo | `invalid github repo %q: must be owner/name with no spaces or qualifier characters` |
| `Since` after `Until` | `since (%s) is after until (%s)` |
| Negative bound | `max_requests must be >= 0, got %d` |

These mirror the existing `config.Resolve` message style verbatim so CLI and MCP error output
stays uniform.

## Added: `ghclient` package

```go
// Option configures a Client. Variadic, so the existing three-argument New
// call sites remain source-compatible.
type Option func(*Client)

// WithRequestBudget caps the provider requests the client may consume and
// enables cost accounting. A ceiling of 0 disables the cap but still accounts.
func WithRequestBudget(ceiling int) Option

func New(token, baseURL string, perPage int, opts ...Option) (*Client, error)   // widened

// CollectActivity gathers a repository's activity for a window.
//
// It returns a populated ActivityResult on every non-fatal path, including
// budget stops, quota exhaustion, and ancestry divergence — the error return is
// reserved for failures that occur before evidence gathering begins.
func (c *Client) CollectActivity(ctx context.Context, q model.ActivityQuery) (model.ActivityResult, error)

// EstimateActivity reports the projected cost without gathering evidence.
func (c *Client) EstimateActivity(ctx context.Context, q model.ActivityQuery) (model.CostReport, error)
```

### Behavior change to `Collect` (User Story 4)

`Collect` currently aborts the whole query when enrichment fails:

```go
if needsDetail(q) {
    if err := c.enrichDetails(ctx, commits, q); err != nil {
        return model.Result{}, err   // <- discards every commit already gathered
    }
}
```

This violates Constitution VI (partial results over blindness) and is corrected here: a budget or
rate-limit stop during enrichment returns the commits gathered so far with an attributable error,
rather than discarding them. The `Result` shape does not change; `Result.Truncated` already exists
to signal clipping.

## Invariants for implementation and review

1. `ActivityResult.SchemaVersion` is always set — never the zero value.
2. `ActivityResult.Cost` is always populated, including on every early-return path.
3. `Correlation.Basis == "observed"` requires `Enriched == true` on every SHA it names.
4. `ChangeSet.Paths` is sorted lexicographically before return.
5. `Disclosures` contains `reference-scoped` and `net-comparison-blindspot` whenever `ChangeSet`
   is non-empty.
6. No exported symbol in `model` imports anything from `internal/`.
7. No code path issues a non-GET request to a provider.
