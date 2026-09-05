// SPDX-License-Identifier: MIT
package mcpserver

import (
	"context"
	"testing"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/model"
)

func TestPRMCPRepoResolution(t *testing.T) {
	old := collectPRs
	t.Cleanup(func() { collectPRs = old })
	collectPRs = func(_ context.Context, _ config.Config, q model.PRQuery) (model.PRResult, error) {
		if q.Author != "" || q.Scope != model.ScopeRepos || q.Since.IsZero() || q.State != model.PRStateAll {
			t.Errorf("query: %+v", q)
		}
		return model.PRResult{}, nil
	}
	h := &handler{cfg: config.Default()}
	if _, _, e := h.getPRs(context.Background(), nil, GetPRsInput{Scope: "repos", Repos: []string{"acme/api"}}); e != nil {
		t.Fatal(e)
	}
}
