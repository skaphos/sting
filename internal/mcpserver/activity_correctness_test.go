// SPDX-License-Identifier: MIT
package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/model"
)

func TestActivityUsesGitHubWithGitLabDefault(t *testing.T) {
	cfg := config.Default()
	cfg.DefaultProvider, cfg.DefaultScope = model.ProviderGitLab, model.ScopeRepos
	h := handler{cfg: cfg}
	swapCollectActivity(t, func(_ context.Context, _ config.Config, q model.ActivityQuery) (model.ActivityResult, error) {
		if q.Provider != model.ProviderGitHub {
			t.Fatalf("activity provider = %s", q.Provider)
		}
		return model.ActivityResult{SchemaVersion: model.ActivitySchemaVersion}, nil
	})
	if _, _, err := h.getRepoActivity(context.Background(), nil, GetRepoActivityInput{Repo: "a/b"}); err != nil {
		t.Fatal(err)
	}
	old := collectCommits
	t.Cleanup(func() { collectCommits = old })
	collectCommits = func(_ context.Context, _ config.Config, q model.Query) (model.Result, error) {
		if q.Provider != model.ProviderGitLab {
			t.Fatalf("commit query provider = %s", q.Provider)
		}
		return model.Result{}, nil
	}
	if _, _, err := h.getCommits(context.Background(), nil, GetCommitsInput{Author: "alice", Repos: []string{"a/b"}}); err != nil {
		t.Fatal(err)
	}
}

func TestActivityTransmitsPartialFailure(t *testing.T) {
	swapCollectActivity(t, func(context.Context, config.Config, model.ActivityQuery) (model.ActivityResult, error) {
		return model.ActivityResult{SchemaVersion: model.ActivitySchemaVersion, Count: 1,
			Commits:     []model.ActivityCommit{{SHA: "head", Message: "retained evidence"}},
			Cost:        model.CostReport{Consumed: 2},
			Disclosures: []model.Disclosure{{Kind: model.DisclosureCollectionFailed, Reason: "compare outage"}},
		}, errors.New("compare outage")
	})
	res, out, err := activityHandler().getRepoActivity(context.Background(), nil, GetRepoActivityInput{Repo: "a/b"})
	if err != nil || res == nil || !res.IsError || out.Count != 1 || out.Cost.Consumed != 2 {
		t.Fatalf("partial result lost: res=%+v out=%+v err=%v", res, out, err)
	}
	if md := res.Content[0].(*mcp.TextContent).Text; !strings.Contains(md, "retained evidence") || !strings.Contains(md, "compare outage") {
		t.Fatalf("partial text lost: %s", md)
	}
}

func TestActivityMCPEstimateIsNotEmptyEvidence(t *testing.T) {
	swapCollectActivity(t, func(_ context.Context, _ config.Config, q model.ActivityQuery) (model.ActivityResult, error) {
		return model.ActivityResult{SchemaVersion: model.ActivitySchemaVersion, EstimateOnly: q.EstimateOnly,
			Disclosures: []model.Disclosure{{Kind: model.DisclosureQuotaExhausted, Reason: "quota stop"}}}, nil
	})
	res, _, err := activityHandler().getRepoActivity(context.Background(), nil, GetRepoActivityInput{Repo: "a/b", EstimateOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	md := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(md, "No evidence was gathered") || !strings.Contains(md, "quota stop") || strings.Contains(md, "No commits in this window") {
		t.Fatalf("bad estimate text: %s", md)
	}
}
