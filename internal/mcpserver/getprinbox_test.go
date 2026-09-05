// SPDX-License-Identifier: MIT
package mcpserver

import (
	"context"
	"testing"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/model"
)

func TestPRInboxMCPIsolatesInvalidWindow(t *testing.T) {
	cfg := config.Default()
	cfg.DefaultWindow = "invalid"
	h := &handler{cfg: cfg}
	old := collectPRInbox
	t.Cleanup(func() { collectPRInbox = old })
	calls := 0
	collectPRInbox = func(_ context.Context, _ config.Config, q model.PRInboxQuery) (model.PRInboxResult, error) {
		calls++
		return model.PRInboxResult{SchemaVersion: model.PRInboxSchemaVersion, Query: q, PRs: []model.PRInboxEntry{}}, nil
	}
	for range 2 {
		res, _, e := h.getPRInbox(context.Background(), nil, GetPRInboxInput{User: "octocat"})
		if e != nil || res.IsError {
			t.Fatalf("inbox blocked: %v", e)
		}
		if _, _, e = h.getPRs(context.Background(), nil, GetPRsInput{Author: "octocat"}); e == nil {
			t.Fatal("invalid activity accepted")
		}
	}
	if calls != 2 {
		t.Fatal("inbox calls lost")
	}
}
