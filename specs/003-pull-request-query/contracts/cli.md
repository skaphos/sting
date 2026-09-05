# Contract: PR CLI Commands

These interfaces are implemented. The existing root query, `sting query`, and
`sting activity` retain their behavior. See [data model](../data-model.md) for result fields.

## Activity — `sting prs`

```text
sting prs [--provider github] [--scope search|repos|org]
          [--author LOGIN] [--repos OWNER/REPO,...] [--org ORG]
          [--since TIME] [--until TIME] [--window DURATION]
          [--state open|merged|closed|all]
          [--time-basis created|updated|merged|closed|any]
          [--role author] [--draft[=true|false] | --no-draft]
          [--max-prs N] [--max-requests N] [-o markdown|json]
```

Search requires an author; repos/org author is optional. Default state is `all`, time basis
`any`, role `author` only when author-filtered, and draft selection unrestricted. `any` selects
opened/merged/unmerged-closed actions, not general updates. State filters current state in
addition to historical matching. `--time-basis closed --state open` can find reopened work.
`--time-basis merged --state open|closed` is rejected as contradictory.

Time parsing matches existing query semantics: UTC normalization; inclusive bounds; explicit
since overrides window; until defaults to captured request time; default window seven days or
configured value. Date-only boundaries are midnight UTC, not whole-day inclusive shortcuts.
Omitting dates never requests an all-time PR queue. History requests require no extra opt-in;
they are authorized core collection and consume the same request budget.

## Inbox — `sting inbox`

```text
sting inbox [--provider github] [--scope search|repos|org]
            [--user LOGIN] [--repos OWNER/REPO,...] [--org ORG]
            [--draft[=true|false] | --no-draft]
            [--max-prs N] [--max-requests N] [-o markdown|json]
```

Omitted user resolves the owner of sting's selected GitHub credential. Explicit user avoids
identity discovery. Open authored, assigned, and directly review-requested PRs are a fixed union;
all inclusion reasons are shown. Drafts are included. There is no date cutoff or current-state
selector. Explicit date, action, author, or role filters are rejected with guidance to use
`sting prs`; the inbox does not inherit their configured activity defaults.

## Shared resolution and configuration

- Existing persistent config/token/base-url/per-page options and dedicated credential precedence
  continue to apply. Empty provider is GitHub for these commands; explicit GitLab is rejected.
- Add only `max_prs` / `STING_MAX_PRS` / `--max-prs` (default 100) to shared configuration.
  Reuse `max_requests` / `STING_MAX_REQUESTS` / `--max-requests` (default 500). Zero disables the
  selected cap; omitted flags inherit config; negative values fail locally.
- Reuse configured scope/targets/format/per-page. Search may combine org and repo restrictions
  as an intersection. Repos and org scopes reject irrelevant explicit target selectors and
  ignore inapplicable default targets. Validate all qualifier inputs against injection.
- `--draft` means draft-only; `--draft=false` means exclude drafts; `--no-draft` is an exclude
  shorthand. Supplying both selector flags is an error; reject `--no-draft=false` with guidance
  to use `--draft`. Absence means unrestricted, not false.
- PR command loading validates applicable typed settings: inbox must not fail because an unused
  activity window is invalid or because the commit provider default is GitLab. This does not
  relax validation for existing standalone commands or bypass config-file parse errors. The
  multi-tool `sting mcp` command uses the [MCP startup validation boundary](mcp.md), so an unused
  activity default cannot prevent inbox tool access.
- Render format is `markdown` or `json`; no positional arguments, estimate, files, reviews,
  stats, diffs, base-branch filter, or mutation flag is added.

## Examples and expected selection

```sh
sting prs --author octocat --window 7d
sting prs --scope repos --repos acme/api --time-basis merged --window 2w -o json
sting prs --scope org --org acme --time-basis closed --since 2026-08-24 --until 2026-08-25
sting inbox --scope org --org acme
sting inbox --user octocat --scope repos --repos acme/api,acme/web --no-draft -o json
```

The first command reports actions on the author's PRs, including closed-unmerged work and
historical closures despite later reopening. The inbox examples include old open work but
exclude completed work. Unrestricted search is explicitly public-only; use org/repo restrictions
for private visibility. Historical search may cap candidates even when few actions match.

## Output and termination

JSON is one complete report on stdout, including a populated partial report before nonzero
collection failure. Diagnostics go to stderr. Validation/client construction errors emit no
fabricated result. Markdown groups by org/repo and shows activity actions or inbox reasons,
selection, limits/costs, and gaps, using only structured facts.

Own caps and disclosed search limits exit 0. Provider errors, identity failure, rate limits,
and cancellation exit 1 while preserving any initialized report. Org-specific skips and
ambiguous history remain disclosed partial results. See [collection contract](provider-collection.md).
CLI contexts keep the existing two-minute timeout; timeout is an attributable incomplete result,
not a claim that the configured request ceiling was reached.
