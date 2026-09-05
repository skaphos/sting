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

// GetPRsInput is the windowed PR tool input; omitted pointer fields inherit defaults.
type GetPRsInput struct {
	Provider    string   `json:"provider,omitempty" jsonschema:"github only"`
	Scope       string   `json:"scope,omitempty" jsonschema:"search, repos, or org"`
	Author      string   `json:"author,omitempty" jsonschema:"PR author login, required for search"`
	Repos       []string `json:"repos,omitempty"`
	Org         string   `json:"org,omitempty"`
	Since       string   `json:"since,omitempty"`
	Until       string   `json:"until,omitempty"`
	Window      string   `json:"window,omitempty"`
	State       string   `json:"state,omitempty" jsonschema:"current open, merged, closed-unmerged, or all (default)"`
	TimeBasis   string   `json:"time_basis,omitempty" jsonschema:"created, updated, merged, closed, or any (default); updated uses latest update only"`
	Role        string   `json:"role,omitempty" jsonschema:"author only"`
	Draft       *bool    `json:"draft,omitempty"`
	MaxPRs      *int     `json:"max_prs,omitempty"`
	MaxRequests *int     `json:"max_requests,omitempty"`
}

var collectPRs = func(ctx context.Context, cfg config.Config, q model.PRQuery) (model.PRResult, error) {
	c, err := commitclient.NewPR(cfg)
	if err != nil {
		return model.PRResult{}, err
	}
	return c.CollectPRs(ctx, q)
}

func (h *handler) getPRs(ctx context.Context, _ *mcp.CallToolRequest, in GetPRsInput) (res *mcp.CallToolResult, out model.PRResult, err error) {
	defer func() {
		if recover() != nil {
			res = errorResult(fmt.Errorf("get_prs: internal error"))
			out = model.PRResult{}
			err = nil
		}
	}()
	q, err := h.cfg.ResolvePRs(config.PRRequest{Provider: in.Provider, Scope: in.Scope, Author: in.Author, Repos: in.Repos, Org: in.Org, Since: in.Since, Until: in.Until, Window: in.Window, State: in.State, TimeBasis: in.TimeBasis, Role: in.Role, Draft: in.Draft, MaxPRs: in.MaxPRs, MaxRequests: in.MaxRequests}, time.Now())
	if err != nil {
		return nil, model.PRResult{}, err
	}
	out, err = collectPRs(ctx, h.cfg, q)
	if out.SchemaVersion == "" {
		return nil, out, err
	}
	return &mcp.CallToolResult{IsError: err != nil, Content: []mcp.Content{&mcp.TextContent{Text: render.PRsMarkdown(out)}}}, out, nil
}
