# Contract: `sting activity` CLI Command

**Feature**: [../spec.md](../spec.md) | **Data model**: [../data-model.md](../data-model.md)

A new subcommand alongside `query`, `mcp`, `auth`, `init`, and `install`. Existing commands are
unchanged.

## Synopsis

```sh
sting activity --repo owner/name [--window 7d | --since DATE --until DATE] [flags]
```

## Flags

| Flag | Type | Default | Maps to |
|---|---|---|---|
| `--repo` | string | *(required)* | `ActivityRequest.Repo` |
| `--ref` | string | default branch | `ActivityRequest.Ref` |
| `--window` | string | config `default_window` (`7d`) | `ActivityRequest.Window` |
| `--since` | string | — | `ActivityRequest.Since` |
| `--until` | string | now | `ActivityRequest.Until` |
| `--author` | string | all authors | `ActivityRequest.Author` |
| `--include-diffs` | bool | `false` | `ActivityRequest.IncludeDiffs` |
| `--max-diff-bytes` | int | config (`60000`) | `ActivityRequest.MaxDiffBytes` |
| `--enrich-commits` | int | `0` | `ActivityRequest.EnrichCommits` |
| `--max-requests` | int | `500` | `ActivityRequest.MaxRequests` |
| `--estimate` | bool | `false` | `ActivityRequest.EstimateOnly` |
| `--format` | string | config `default_format` (`markdown`) | `render.Format` |

Flag names, precedence (flags > env > file > defaults), and the `--format` values (`markdown`,
`json`) match the existing `query` command so the two feel like one tool.

`--max-requests` is also added to `sting query`, which is how User Story 4 reaches the CLI.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | A result was produced — **including** a budget-bounded, quota-stopped, or diverged result |
| `1` | The query could not run: invalid input, unreadable repository, or a credential failure |

A bounded result exits `0` because evidence was produced and disclosed. Exiting non-zero would
tell a script the query failed when it did not, and would encourage discarding usable partial
evidence — the opposite of Constitution VI.

## Output

Markdown by default; `--format json` emits `model.ActivityResult` verbatim as the contract.

The Markdown view, in order: resolved query and boundaries → commits (newest first, full
messages) → change set → cost report → disclosures. Disclosures render as a visible section, not a
footnote.

### Worked example

```sh
$ sting activity --repo skaphos/sting --window 7d
```

```text
# Activity: skaphos/sting

**Reference**: main
**Window**: 2026-07-18T00:00:00Z .. 2026-07-25T00:00:00Z (by committer date)
**Boundaries**: 1ce380e..1ae89e4 (base: parent-of-earliest, status: ahead)

## Commits (5)
...

## Change set (23 files, +1,204 / -318)
...

## Cost
Requests: 8 consumed of 500 ceiling. Quota: 4,921 of 5,000 remaining, resets 14:32Z.

## Disclosures
- reference-scoped: This covers `main` only. Work on other branches, forks, or unmerged
  pull requests is not included.
- net-comparison-blindspot: The change set compares the window's start and end states. A file
  created and deleted inside the window, or edited then reverted, does not appear.
```

### Estimate mode

```sh
$ sting activity --repo skaphos/sting --window 7d --estimate
```

```text
Estimated cost: 8 provider requests (5 commits in window across 1 page).
Ceiling: 500. Quota: 4,929 of 5,000 remaining, resets 14:32Z.
No evidence was gathered. Re-run without --estimate to collect it.
```

The estimate itself consumes one request (research R4), which is reported rather than hidden.

## Testing notes

`internal/cli` carries a documented 60% coverage floor (`scripts/check-coverage.sh`) because of
its interactive surface. The new command must keep flag parsing and request construction thin, so
that the logic worth testing lives in `config.ResolveActivity` and `ghclient.CollectActivity`,
both of which are held to the standard 80% gate.
