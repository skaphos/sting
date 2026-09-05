// SPDX-License-Identifier: MIT
package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/internal/commitclient"
	"github.com/skaphos/sting/internal/render"
	"github.com/skaphos/sting/model"
)

// GetPRInboxInput cannot express activity filters; the SDK rejects unknown properties.
type GetPRInboxInput struct {
	Provider    string   `json:"provider,omitempty" jsonschema:"github only"`
	Scope       string   `json:"scope,omitempty" jsonschema:"search, repos, or org"`
	Repos       []string `json:"repos,omitempty"`
	Org         string   `json:"org,omitempty"`
	User        string   `json:"user,omitempty" jsonschema:"login; omitted resolves sting's authenticated user"`
	Draft       *bool    `json:"draft,omitempty"`
	MaxPRs      *int     `json:"max_prs,omitempty"`
	MaxRequests *int     `json:"max_requests,omitempty"`
}

var collectPRInbox = func(ctx context.Context, cfg config.Config, q model.PRInboxQuery) (model.PRInboxResult, error) {
	c, err := commitclient.NewPR(cfg)
	if err != nil {
		return model.PRInboxResult{}, err
	}
	return c.CollectPRInbox(ctx, q)
}

func (h *handler) getPRInbox(ctx context.Context, _ *mcp.CallToolRequest, in GetPRInboxInput) (res *mcp.CallToolResult, out model.PRInboxResult, err error) {
	defer func() {
		if recover() != nil {
			res = errorResult(fmt.Errorf("get_pr_inbox: internal error"))
			out = model.PRInboxResult{}
			err = nil
		}
	}()
	q, err := h.cfg.ResolvePRInbox(config.PRInboxRequest{Provider: in.Provider, Scope: in.Scope, Repos: in.Repos, Org: in.Org, User: in.User, Draft: in.Draft, MaxPRs: in.MaxPRs, MaxRequests: in.MaxRequests})
	if err != nil {
		return nil, model.PRInboxResult{}, err
	}
	out, err = collectPRInbox(ctx, h.cfg, q)
	if out.SchemaVersion == "" {
		return nil, out, err
	}
	return &mcp.CallToolResult{IsError: err != nil, Content: []mcp.Content{&mcp.TextContent{Text: render.PRInboxMarkdown(out)}}}, out, nil
}
