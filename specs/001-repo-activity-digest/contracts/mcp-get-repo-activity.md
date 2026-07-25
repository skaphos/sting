# Contract: `get_repo_activity` MCP Tool

**Feature**: [../spec.md](../spec.md) | **Data model**: [../data-model.md](../data-model.md)

A second read-only tool alongside `get_commits`. The existing tool's schema and behavior are
unchanged (FR-026).

## Registration

```go
mcp.AddTool(server, &mcp.Tool{
    Name: "get_repo_activity",
    Description: "Summarize what happened in a GitHub repository over a time window. " +
        "Returns the window's commits with full messages, the aggregate per-file change " +
        "set derived from comparing the window's boundary states, and a cost report. " +
        "Designed to stay cheap: the request count does not grow per commit. Use this " +
        "instead of get_commits with include_diffs when you want a repository's story " +
        "rather than one person's commits.",
    Annotations: &mcp.ToolAnnotations{
        ReadOnlyHint:  true,
        OpenWorldHint: boolPtr(true),
    },
}, h.getRepoActivity)
```

Both this tool and `get_commits` are registered from one definition slice, from which
`ReadOnlyTools()` is derived (research R6), so the installer auto-approve list cannot drift from
what the server actually exposes.

## Input schema

```go
type GetRepoActivityInput struct {
    Repo          string `json:"repo" jsonschema:"repository as owner/name; required"`
    Ref           string `json:"ref,omitempty" jsonschema:"branch or tag to examine; defaults to the repository's default branch"`
    Since         string `json:"since,omitempty" jsonschema:"start of window, RFC3339 or YYYY-MM-DD; if omitted, window is used"`
    Until         string `json:"until,omitempty" jsonschema:"end of window, RFC3339 or YYYY-MM-DD; defaults to now"`
    Window        string `json:"window,omitempty" jsonschema:"look-back window when since is omitted, e.g. 7d, 2w, 48h; defaults to the server default"`
    Author        string `json:"author,omitempty" jsonschema:"optional: narrow the commit list to one author; the change set still covers all authors and says so"`
    IncludeDiffs  *bool  `json:"include_diffs,omitempty" jsonschema:"opt in to bounded patch text in the change set; defaults to false"`
    MaxDiffBytes  int    `json:"max_diff_bytes,omitempty" jsonschema:"patch byte cap when include_diffs is true; defaults to server config"`
    EnrichCommits *int   `json:"enrich_commits,omitempty" jsonschema:"fetch per-commit file data for this many of the newest commits, enabling observed (rather than inferred) path attribution; costs one request each; defaults to 0"`
    MaxRequests   *int   `json:"max_requests,omitempty" jsonschema:"cap provider requests for this call; defaults to the server default of 500; set 0 only for an intentional uncapped run"`
    EstimateOnly  bool   `json:"estimate_only,omitempty" jsonschema:"report projected cost and remaining quota without gathering evidence"`
}
```

`Provider` is deliberately absent — the tool is GitHub-only (FR-025), and offering a parameter
whose only other value is an error would mislead the agent.

The `*bool` / `*int` pointer convention matches `GetCommitsInput`: it distinguishes an explicit
`false`/`0` from an omitted field, which matters for `max_requests` where `0` means "uncapped".

## Output

- **Structured**: `model.ActivityResult` (the contract).
- **Text content**: a Markdown rendering of the same result (a view, never a superset —
  Principle II).

The Markdown view leads with the resolved query and boundaries, then the commit list, then the
change set, and ends with the cost report and disclosures. Disclosures are rendered as visible
prose rather than a footnote, because an agent that misses `net-comparison-blindspot` will
overstate what the evidence shows.

## Error handling

Follows the `getCommits` pattern exactly:

- A deferred `recover()` converts a panic into a tool-level error result, so a panic cannot kill
  the long-lived stdio server.
- Ordinary errors are returned as the handler's error value, not folded into a zero-value
  structured payload — the go-sdk marshals `out` only when `err` is nil, so a failure never emits
  a schema-shaped empty result alongside `IsError` text.

| Condition | Response |
|---|---|
| Invalid input (bad repo, bad window, non-GitHub) | Handler error with the `ResolveActivity` message |
| Repository unreadable | Handler error naming the repository and the reason |
| Budget or quota stop | **Success** with a populated result plus a disclosure — not an error |
| Ancestry diverged | **Success** with an empty change set plus an `ancestry-diverged` disclosure |

That third row is the important one: a bounded result is a *result*, not a failure. Returning an
error there would discard the evidence already gathered, which Constitution VI forbids.

## Installer impact

`ReadOnlyTools()` returns `["get_commits", "get_repo_activity"]`, so
`mcpinstall.ClaudePermissionsSnippet` (`internal/cli/install.go:187`) emits both entries. Users who
installed before this change get the second tool on re-install; the tool remains functional
without auto-approval, just prompted.
