# Phase 1 Data Model: Repository Activity Digest

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Research**:
[research.md](./research.md)

All types live in `model/activity.go` — the same public package as the existing contract, per
ADR 0004, but as **distinct types**. `model.Result`, `model.Commit`, `model.Query`, and
`model.SchemaVersion` are not modified by this feature.

## Schema version

```go
// ActivitySchemaVersion pins the ActivityResult contract, independently of
// SchemaVersion which pins Result. Bump on any breaking change to
// ActivityResult or the types it contains.
const ActivitySchemaVersion = "sting.activity.skaphos.io/v1"

// DefaultMaxRequests bounds the provider requests a single query may consume.
// Fixed by design: deriving it from remaining quota would make the same request
// return different results on different runs (FR-012a).
const DefaultMaxRequests = 500
```

---

## ActivityQuery

The resolved, normalized request. Produced once by `config.ResolveActivity`; never mutated
downstream (Principle III).

| Field | Type | Validation | Requirement |
|---|---|---|---|
| `Provider` | `Provider` | MUST be `github`; anything else rejected at resolve | FR-025 |
| `Repo` | `string` | `owner/name`, validated by the existing `validGitHubRepo` | FR-001 |
| `Ref` | `string` | Branch or tag; empty means the repository's default branch | FR-005 |
| `Since`, `Until` | `time.Time` | Normalized to UTC once at the boundary; `Since < Until` | FR-003, FR-004 |
| `Author` | `string` | Optional narrowing filter; empty means all authors | FR-002 |
| `IncludeDiffs` | `bool` | Patch text in the change set, bounded by `MaxDiffBytes` | FR-010 |
| `MaxDiffBytes` | `int` | `>= 0`; `0` uses `DefaultMaxDiffBytes` | FR-019 |
| `EnrichCommits` | `int` | Size of the opt-in per-commit detail subset; `0` disables | FR-010 |
| `MaxRequests` | `int` | `>= 0`; `0` disables the ceiling; defaults to `DefaultMaxRequests` | FR-012 |
| `EstimateOnly` | `bool` | Return a `CostReport` and stop, gathering no evidence | FR-011 |

**Note on `Author`**: filtering is applied server-side on the commit listing, but the change set
is *not* author-filtered — a boundary comparison has no notion of authorship. When `Author` is set
and a change set is produced, a disclosure states that the change set covers all authors in the
window. This is a real limitation of the cheap path and is disclosed rather than hidden.

---

## ActivityResult

The top-level returned type. Every field exists to make the result re-derivable (FR-020).

| Field | Type | Notes | Requirement |
|---|---|---|---|
| `SchemaVersion` | `string` | Always `ActivitySchemaVersion` | FR-024 |
| `GeneratedAt` | `time.Time` | The only field allowed to differ between identical runs | FR-023 |
| `Provider` | `Provider` | Always `github` in v1 | FR-020 |
| `Repo` | `string` | `owner/name` as resolved | FR-020 |
| `Ref` | `string` | The reference actually compared, never empty in output | FR-005 |
| `Since`, `Until` | `time.Time` | Normalized window, echoed | FR-004 |
| `WindowDateBasis` | `string` | Which commit date bounded the window: `committer` or `author` | R2, FR-018 |
| `Boundaries` | `Boundaries` | Resolved base and head | FR-008 |
| `Count` | `int` | `len(Commits)` | — |
| `Commits` | `[]ActivityCommit` | Newest first, provider order preserved | FR-006 |
| `ChangeSet` | `ChangeSet` | Aggregate per-path delta | FR-007 |
| `Correlations` | `[]Correlation` | Path ↔ commit links, each with a basis | FR-017 |
| `Cost` | `CostReport` | Always populated, including on failure paths | FR-014 |
| `Disclosures` | `[]Disclosure` | Empty when nothing was bounded or degraded | FR-019 |

---

## Boundaries

```go
type Boundaries struct {
    BaseSHA     string `json:"base_sha"`               // parent of earliest in-window commit
    HeadSHA     string `json:"head_sha"`               // latest in-window commit
    BaseSource  string `json:"base_source"`            // "parent-of-earliest" | "repository-root"
    Status      string `json:"status"`                 // "ahead" | "identical" | "diverged" | "behind"
    SharedRoot  bool   `json:"shared_ancestry"`        // false => change set suppressed
}
```

| Rule | Behavior | Requirement |
|---|---|---|
| Earliest in-window commit has a parent | `BaseSHA` = first parent; `BaseSource` = `parent-of-earliest` | FR-008 |
| Earliest in-window commit is the root commit | `BaseSHA` = empty; `BaseSource` = `repository-root`; change set is everything since inception | FR-008b |
| Window contains no commits | `BaseSHA == HeadSHA`; `Status` = `identical`; empty change set; **not an error** | Edge case |
| `Status == "diverged"` | `SharedRoot` = false; `ChangeSet` left empty; an `ancestry-diverged` disclosure is emitted | FR-009 |

Resolution is by ancestry, never by timestamp proximity (FR-008a).

---

## ActivityCommit

Deliberately a distinct type from `model.Commit` rather than a reuse, because the window query
carries two fields `Commit` does not and must not gain (`Commit` is pinned by `SchemaVersion`).

| Field | Type | Notes |
|---|---|---|
| `SHA`, `Repo`, `Author`, `AuthorName`, `Email`, `URL` | `string` | Same semantics as `model.Commit` |
| `Message` | `string` | Full body, not just the summary |
| `AuthorDate` | `time.Time` | Git author date |
| `CommitterDate` | `time.Time` | Git committer date — **new**; see R2 |
| `ParentSHAs` | `[]string` | **New**; first element is the first parent |
| `Enriched` | `bool` | True only if per-commit detail was actually fetched |
| `Files` | `[]File` | Reuses the existing `model.File`; populated only when `Enriched` |

`Summary()` returns the first line, matching `model.Commit.Summary()`.

---

## ChangeSet and ChangedPath

```go
type ChangeSet struct {
    Paths          []ChangedPath `json:"paths"`
    TotalAdditions int           `json:"total_additions"`
    TotalDeletions int           `json:"total_deletions"`
    Truncated      bool          `json:"truncated,omitempty"` // provider file cap hit
}

type ChangedPath struct {
    Path           string `json:"path"`
    PreviousPath   string `json:"previous_path,omitempty"` // renames
    Status         string `json:"status"`                  // added|modified|removed|renamed
    Additions      int    `json:"additions"`
    Deletions      int    `json:"deletions"`
    Patch          string `json:"patch,omitempty"`
    PatchTruncated bool   `json:"patch_truncated,omitempty"`
}
```

`Paths` is sorted lexicographically by `Path` so identical upstream state yields byte-identical
output (FR-023) — provider ordering is not guaranteed stable.

---

## Correlation

```go
type Correlation struct {
    Path    string   `json:"path"`
    SHAs    []string `json:"shas"`              // sorted; empty means unattributed
    Basis   string   `json:"basis"`             // see the table below
    Rule    string   `json:"rule,omitempty"`    // populated for inferred bases
}
```

| `Basis` | Meaning | Source |
|---|---|---|
| `observed` | Per-commit file data was fetched and lists this path | Enriched subset only |
| `inferred` | Derived by a declared rule; `Rule` names it | `path-mention` or `scope-match` |
| *(absent from list)* | No rule matched; the path stays unattributed | Honest gap, per R8 |

A correlation MUST NOT carry `observed` unless `ActivityCommit.Enriched` is true for every SHA it
names. This is the single invariant behind FR-017 and is the highest-value unit test in the
feature.

---

## CostReport

```go
type CostReport struct {
    Estimated      int        `json:"estimated"`                 // 0 if no estimate was run
    Consumed       int        `json:"consumed"`
    Ceiling        int        `json:"ceiling"`                   // 0 means disabled
    QuotaRemaining int        `json:"quota_remaining"`
    QuotaLimit     int        `json:"quota_limit"`
    QuotaResetsAt  time.Time  `json:"quota_resets_at,omitempty"`
}
```

Always populated, including on failure paths — a query that stopped early must still report what
it spent (FR-014). Quota fields come from the rate headers captured by the transport (R3).

---

## Disclosure

```go
type Disclosure struct {
    Kind       string `json:"kind"`
    Reason     string `json:"reason"`
    NextAction string `json:"next_action,omitempty"`
}
```

| `Kind` | Emitted when | Requirement |
|---|---|---|
| `budget-bounded` | The ceiling stopped the query | FR-013 |
| `quota-exhausted` | The provider rate limit stopped it | FR-015 |
| `provider-capped` | The comparison hit the 250-commit or 300-file cap | FR-019, R7 |
| `patch-truncated` | Patch text exceeded `MaxDiffBytes` | FR-019 |
| `ancestry-diverged` | Boundaries do not share ancestry | FR-009 |
| `net-comparison-blindspot` | **Always, whenever a change set is produced** | FR-018 |
| `reference-scoped` | **Always** — names the compared reference | FR-018 |
| `author-filter-not-applied` | `Author` was set and a change set was produced | ActivityQuery note |
| `enrichment-partial` | The enrichment subset was smaller than requested | FR-010 |

The two marked **always** are unconditional: FR-018 requires the blind spots to be stated whenever
a boundary comparison is presented, not only when something went wrong. A clean result still
carries them.

---

## Relationships

```text
ActivityQuery ──resolves to──> ActivityResult
                                 ├── Boundaries      (1)
                                 ├── ActivityCommit  (0..n, newest first)
                                 ├── ChangeSet       (1) ──> ChangedPath (0..n, path-sorted)
                                 ├── Correlation     (0..n) ──references──> ChangedPath.Path
                                 │                            └──references──> ActivityCommit.SHA
                                 ├── CostReport      (1, always present)
                                 └── Disclosure      (0..n; ≥2 whenever a ChangeSet exists)
```

## State transitions

The query has no persisted lifecycle (Principle VII). It has one execution path with four defined
terminal states, and **all four return a populated `ActivityResult`** — none returns a bare error:

| Terminal state | `ChangeSet` | `Disclosures` | Error returned |
|---|---|---|---|
| Complete | Populated | The two unconditional ones | none |
| Estimate only | Empty | none | none |
| Budget or quota stop | Whatever was gathered | `budget-bounded` or `quota-exhausted` | none |
| Ancestry diverged | Empty | `ancestry-diverged` | none |

A hard error — repository unreadable, credential rejected, request invalid — is the only path that
returns a Go error rather than a result, and it is returned before evidence gathering begins.
