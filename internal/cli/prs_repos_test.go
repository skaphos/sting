// SPDX-License-Identifier: MIT
package cli

import (
	"context"
	"testing"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/model"
)

func TestPRCLIRepoResolution(t *testing.T) {
	seedValidConfig(t)
	old := collectPRActivity
	t.Cleanup(func() { collectPRActivity = old })
	collectPRActivity = func(_ context.Context, _ config.Config, q model.PRQuery) (model.PRResult, error) {
		if q.Author != "" || q.Scope != model.ScopeRepos || q.Since.IsZero() || q.Until.IsZero() || q.State != model.PRStateAll {
			t.Errorf("query: %+v", q)
		}
		return model.PRResult{}, nil
	}
	cmd, _, _ := newCmd()
	cmd.SetContext(context.Background())
	registerPRFlags(cmd)
	if e := cmd.ParseFlags([]string{"--scope", "repos", "--repos", "acme/api"}); e != nil {
		t.Fatal(e)
	}
	if e := runPRs(cmd, nil); e != nil {
		t.Fatal(e)
	}
}
