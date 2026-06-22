// SPDX-License-Identifier: MIT
// Package mcpserver exposes the commit-query capability as an MCP tool over a
// stdio transport, so an LLM agent can ask for an author's recent commits.
package mcpserver

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/internal/commitclient"
	"github.com/skaphos/sting/internal/render"
	"github.com/skaphos/sting/model"
)

// GetCommitsInput is the argument schema for the get_commits tool. The
// jsonschema descriptions are surfaced to the calling agent.
type GetCommitsInput struct {
	Provider     string   `json:"provider,omitempty" jsonschema:"source control provider: github (default) or gitlab"`
	Author       string   `json:"author" jsonschema:"provider username or author string whose commits to retrieve"`
	Since        string   `json:"since,omitempty" jsonschema:"start of window, RFC3339 or YYYY-MM-DD; if omitted, window is used"`
	Until        string   `json:"until,omitempty" jsonschema:"end of window, RFC3339 or YYYY-MM-DD; defaults to now"`
	Window       string   `json:"window,omitempty" jsonschema:"look-back window when since is omitted, e.g. 7d, 2w, 48h; defaults to the server default"`
	Scope        string   `json:"scope,omitempty" jsonschema:"discovery scope: search (author search; global/public-only unless scoped by org or repos), repos (explicit repo list), or org (enumerate an org's repos; most complete for private orgs)"`
	Repos        []string `json:"repos,omitempty" jsonschema:"repository/project targets; required for scope=repos, and narrows GitHub scope=search to those repos (incl. private with access)"`
	Org          string   `json:"org,omitempty" jsonschema:"organization or GitLab group; required for scope=org, and scopes GitHub scope=search into that org (reaches private repos the token can access)"`
	IncludeStats bool     `json:"include_stats,omitempty" jsonschema:"fetch per-commit line additions/deletions; GitHub uses extra API calls, GitLab uses commit-list stats"`
	IncludeFiles bool     `json:"include_files,omitempty" jsonschema:"fetch per-file change summaries; uses extra commit-detail API calls"`
	IncludeDiffs bool     `json:"include_diffs,omitempty" jsonschema:"fetch bounded patch text for changed files; implies include_files and can be token-heavy"`
	MaxDiffBytes int      `json:"max_diff_bytes,omitempty" jsonschema:"per-commit patch byte cap when include_diffs is true; defaults to server config"`
	IncludePRs   bool     `json:"include_prs,omitempty" jsonschema:"also discover commits on open pull-request branches (scope=repos or org, GitHub only); finds unmerged work that commit search and branch listing miss, at the cost of extra API calls"`
}

// handler holds the dependencies shared across tool calls.
type handler struct {
	cfg config.Config
}

// New builds an MCP server exposing the get_commits tool, configured from cfg.
func New(cfg config.Config) (*mcp.Server, error) {
	h := &handler{cfg: cfg}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sting",
		Version: "0.1.0",
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_commits",
		Description: "Retrieve a GitHub or GitLab user's commits over a time window. " +
			"Returns the commits as structured data plus a Markdown summary so " +
			"you can describe what the person has been working on.",
		// The tool only reads from provider APIs; it never mutates anything.
		// OpenWorldHint is true because it reaches an external service.
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(true),
		},
	}, h.getCommits)

	return server, nil
}

// ReadOnlyTools lists the names of tools whose ReadOnlyHint is true. It is the
// single source of truth for which tools an installer may safely auto-approve,
// so the install permissions snippet cannot drift from what the server marks
// read-only. Every tool sting exposes is read-only by design.
func ReadOnlyTools() []string {
	return []string{"get_commits"}
}

func boolPtr(b bool) *bool { return &b }

func (h *handler) getCommits(ctx context.Context, _ *mcp.CallToolRequest, in GetCommitsInput) (*mcp.CallToolResult, model.Result, error) {
	req := config.Request{
		Provider: in.Provider,
		Author:   in.Author,
		Since:    in.Since,
		Until:    in.Until,
		Window:   in.Window,
		Scope:    in.Scope,
		Repos:    in.Repos,
		Org:      in.Org,
	}
	if in.IncludeStats {
		req.IncludeStats = &in.IncludeStats
	}
	if in.IncludeFiles {
		req.IncludeFiles = &in.IncludeFiles
	}
	if in.IncludeDiffs {
		req.IncludeDiffs = &in.IncludeDiffs
	}
	if in.MaxDiffBytes != 0 {
		req.MaxDiffBytes = &in.MaxDiffBytes
	}
	if in.IncludePRs {
		req.IncludePullRequests = &in.IncludePRs
	}

	q, err := h.cfg.Resolve(req, time.Now())
	if err != nil {
		return errorResult(err), model.Result{}, nil
	}

	client, err := commitclient.New(h.cfg, q.Provider)
	if err != nil {
		return errorResult(err), model.Result{}, nil
	}

	result, err := client.Collect(ctx, q)
	if err != nil {
		return errorResult(err), model.Result{}, nil
	}

	md := render.Markdown(result)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: md}},
	}, result, nil
}

// errorResult reports a tool-level error back to the agent as text so it can
// self-correct, rather than surfacing a protocol error.
func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
	}
}
