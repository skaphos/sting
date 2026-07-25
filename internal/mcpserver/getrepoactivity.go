// SPDX-License-Identifier: MIT
package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/internal/commitclient"
	"github.com/skaphos/sting/internal/render"
	"github.com/skaphos/sting/model"
)

// GetRepoActivityInput is the argument schema for the get_repo_activity tool.
//
// Provider is deliberately absent: the tool is GitHub-only, and offering a
// parameter whose only other value is an error would mislead the agent.
//
// The *bool / *int pointer convention matches GetCommitsInput — it distinguishes
// an explicit false/0 from an omitted field, which matters for max_requests
// where 0 means "uncapped" rather than "unset".
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

// collectActivity resolves an activity client and gathers the result. It is a
// package variable so tests can substitute a panic-inducing implementation to
// exercise the recovery path without a real provider client.
var collectActivity = func(ctx context.Context, cfg config.Config, q model.ActivityQuery) (model.ActivityResult, error) {
	client, err := commitclient.NewActivity(cfg, q)
	if err != nil {
		return model.ActivityResult{}, err
	}
	return client.CollectActivity(ctx, q)
}

// getRepoActivity follows the getCommits pattern exactly.
//
// The go-sdk dispatches handlers in their own goroutine with no panic recovery,
// so an unrecovered panic anywhere in the Resolve->client->render chain would
// crash the whole long-lived stdio server; the deferred recover converts it into
// a tool-level error result.
//
// Ordinary errors are returned as the handler's error value rather than folded
// into a success result: the go-sdk marshals the structured Out value only when
// the error is nil, so a failure never emits a schema-shaped, zero-value payload
// alongside the IsError text.
//
// A budget or quota stop is deliberately NOT an error — CollectActivity returns
// a populated result plus a disclosure, and surfacing that as a failure would
// discard evidence the caller can use.
func (h *handler) getRepoActivity(ctx context.Context, _ *mcp.CallToolRequest, in GetRepoActivityInput) (res *mcp.CallToolResult, out model.ActivityResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = errorResult(fmt.Errorf("get_repo_activity: internal error: %v", r))
			out = model.ActivityResult{}
			err = nil
		}
	}()

	req := config.ActivityRequest{
		Repo:         in.Repo,
		Ref:          in.Ref,
		Since:        in.Since,
		Until:        in.Until,
		Window:       in.Window,
		Author:       in.Author,
		EstimateOnly: in.EstimateOnly,
	}
	// Diffs are opt-in for MCP calls independent of server config, because they
	// add output volume the agent did not ask for.
	includeDiffs := false
	if in.IncludeDiffs != nil {
		includeDiffs = *in.IncludeDiffs
	}
	req.IncludeDiffs = &includeDiffs
	if in.MaxDiffBytes != 0 {
		req.MaxDiffBytes = &in.MaxDiffBytes
	}
	if in.EnrichCommits != nil {
		req.EnrichCommits = in.EnrichCommits
	}
	if in.MaxRequests != nil {
		req.MaxRequests = in.MaxRequests
	}

	q, rerr := h.cfg.ResolveActivity(req, time.Now())
	if rerr != nil {
		return nil, model.ActivityResult{}, rerr
	}

	result, cerr := collectActivity(ctx, h.cfg, q)
	if cerr != nil {
		return nil, model.ActivityResult{}, cerr
	}

	md := render.ActivityMarkdown(result)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: md}},
	}, result, nil
}
