---
name: "speckit-taskstoissues"
description: "Convert existing tasks into actionable, dependency-ordered GitHub issues for the feature based on available design artifacts."
argument-hint: "Optional granularity override: phase | story | task"
compatibility: "Requires spec-kit project structure with .specify/ directory"
metadata:
  author: "skaphos"
  source: "skaphos-resources/llm_resources/speckit/skills/speckit-taskstoissues"
  upstream: "github-spec-kit templates/commands/taskstoissues.md"
  overrides: "Groups tasks into phase/story issues instead of one issue per task"
user-invocable: true
disable-model-invocation: false
---

## User Input

```text
$ARGUMENTS
```

You **MUST** consider the user input before proceeding (if not empty). If it is one of
`phase`, `story`, or `task`, treat it as a granularity override for this run.

---

## Why this skill overrides the upstream one

Upstream `speckit.taskstoissues` creates **one GitHub issue per task**. A `tasks.md` produced by
`/speckit-tasks` routinely contains 40–80 tasks, so the upstream behavior floods the tracker with
dozens of single-edit issues that no one triages, assigns, or closes individually. The issue list
stops being a board and becomes a transcript of the plan.

This version groups tasks into **deliverable-sized issues** — by default one per phase, which maps
to the setup / foundational / per-user-story / polish structure `/speckit-tasks` already produces.
Individual tasks become checklist items inside their group, so nothing is lost: the granularity
still exists, it just lives in a checklist rather than in the issue count.

The upstream one-per-task behavior remains available via `granularity: task` for teams that want it.

---

## Configuration

Read `.specify/extensions.yml` if it exists. Beyond the standard `hooks:` key, this skill honors a
`settings:` block:

```yaml
settings:
  taskstoissues:
    granularity: phase     # phase (default) | story | task
    max_issues: 12         # refuse to exceed this without explicit confirmation
    labels: []             # labels applied to every created issue (must already exist)
    title_prefix: ""       # optional prefix, e.g. "[sting]"
```

Resolution order, highest first: `$ARGUMENTS` → `settings.taskstoissues` → the defaults above.

If the file is missing or unparseable, use the defaults and continue — never fail the command over
configuration.

### Granularity modes

| Mode | One issue per | Typical count | Use when |
|---|---|---|---|
| `phase` *(default)* | `## Phase N` heading in `tasks.md` | 5–8 | Almost always. Matches how `/speckit-tasks` already structures work. |
| `story` | user story (`[US1]`…), plus one for un-storied setup/foundational/polish tasks | 4–7 | You want the board to mirror the spec's user stories exactly. |
| `task` | task | 40–80 | Rarely. A team that genuinely assigns and tracks individual edits. |

---

## Pre-Execution Checks

Check `.specify/extensions.yml` for `hooks.before_taskstoissues` and dispatch per the standard
Spec Kit hook protocol: skip entries with `enabled: false`; do not evaluate `condition`
expressions yourself; convert dots in command names to hyphens (`speckit.git.commit` →
`/speckit-git-commit`); emit `EXECUTE_COMMAND:` and actually invoke mandatory (`optional: false`)
hooks, waiting for each to finish. If the file or key is absent, skip silently.

---

## Outline

1. Run `.specify/scripts/bash/check-prerequisites.sh --json --require-tasks --include-tasks` from
   the repository root. Parse `FEATURE_DIR` and `AVAILABLE_DOCS`. Paths are absolute.

2. **IF EXISTS**: load `.specify/memory/constitution.md` for principles and governance
   constraints that belong in each issue body (testing requirements, commit signing, PR-only).

3. Resolve the tasks file from `FEATURE_DIR` and read it.

4. Confirm the remote is GitHub:

   ```bash
   git config --get remote.origin.url
   ```

   > [!CAUTION]
   > **Only proceed if the remote is a GitHub URL.** Derive `owner/repo` from it and create issues
   > **only** in that repository. Never create issues in any other repository under any
   > circumstances.

5. **Parse `tasks.md` into groups.**

   Task lines look like `- [ ] T001 [P] [US1] Description with file path`. Strip the checkbox,
   capture the ID (`T` plus three digits), and record the `[P]` and `[US#]` markers separately from
   the description. Track the enclosing `## Phase N: ...` heading for each task.

   Then group according to the resolved granularity. Every task MUST land in exactly one group —
   verify total grouped == total parsed before creating anything, and abort with a clear message if
   they differ. Silently dropping a task is worse than failing.

6. **Apply the count guardrail.** If the number of groups exceeds `max_issues`, do not create
   anything. Report the count, the configured limit, and the three ways forward: coarsen
   granularity, raise `max_issues`, or confirm explicitly. This is what stops a misconfigured run
   from flooding a tracker.

7. **Deduplicate.** Build each group's stable key — the feature directory ID plus the group
   identifier, e.g. `001-repo-activity-digest/phase-3`. Embed it in every issue body as an HTML
   comment marker:

   ```html
   <!-- speckit:feature=001-repo-activity-digest group=phase-3 -->
   ```

   Before creating, list existing issues (open **and** closed — omit any state filter) and match
   against these markers, falling back to the issue title when a body has no marker. Skip groups
   that already have an issue and report each skip (`phase-3 already has an issue (#113), skipping`).
   Paginate with `perPage: 100` and stop as soon as every key is accounted for, so re-runs on a repo
   with a long issue history stay cheap.

   In `task` mode, dedupe on the task ID with the word-boundary pattern `\bT\d{3}\b`, so `ST001` and
   `T0010` do not match by mistake.

8. **Create one issue per remaining group**, with this body structure:

   - The stable marker comment (step 7)
   - **Goal** — what the group delivers, from the spec's user story or the phase purpose
   - **Independent test** — how to verify this group works on its own, from `tasks.md`
   - **Depends on** — other groups, cross-referenced by issue number once known
   - **Tasks (N)** — every task as a markdown checkbox: `- [ ] **T004** \`[P]\` — description`
   - **Requirements** — testing and governance constraints from the constitution
   - **Design context** — links to `spec.md`, `plan.md`, `research.md`, `data-model.md`,
     `contracts/`, `quickstart.md` on the current branch

   Titles are `<title_prefix><group label>: <short description>`, kept under 120 characters.
   Because GitHub caps titles at 256 characters, truncate at a clause boundary and keep the full
   text in the body — never emit a title cut mid-token or with unbalanced backticks.

   Create groups in dependency order so earlier issue numbers can be referenced by later bodies.
   Space creation by ~1 second to stay under GitHub's content-creation secondary rate limit. If a
   body references a group created later, update it after the fact rather than leaving a dangling
   placeholder.

9. **Report**: issues created with numbers, groups skipped as duplicates, the granularity used, and
   confirmation that grouped task count equals parsed task count.

---

## Post-Execution Checks

Check `.specify/extensions.yml` for `hooks.after_taskstoissues` and dispatch per the same protocol
as the pre-execution checks. If absent, skip silently.

---

## Done When

- [ ] Remote verified as GitHub and issues created only in the matching repository
- [ ] Every parsed task appears in exactly one issue
- [ ] Group count within `max_issues`, or explicitly confirmed
- [ ] Existing issues detected and skipped rather than duplicated
- [ ] Extension hooks dispatched or skipped per protocol
- [ ] Completion reported with issue numbers, skips, and granularity used
