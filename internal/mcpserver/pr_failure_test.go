// SPDX-License-Identifier: MIT
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/model"
)

func TestPRInboxMCPFailureAndPanic(t *testing.T) {
	old := collectPRInbox
	t.Cleanup(func() { collectPRInbox = old })
	h := &handler{cfg: config.Default()}
	collectPRInbox = func(context.Context, config.Config, model.PRInboxQuery) (model.PRInboxResult, error) {
		return model.PRInboxResult{SchemaVersion: model.PRInboxSchemaVersion, PRs: []model.PRInboxEntry{}, Truncated: true}, errors.New("fixture failure")
	}
	res, out, e := h.getPRInbox(context.Background(), nil, GetPRInboxInput{User: "octocat"})
	if e != nil || !res.IsError || out.SchemaVersion == "" {
		t.Fatal("partial failure discarded")
	}
	collectPRInbox = func(context.Context, config.Config, model.PRInboxQuery) (model.PRInboxResult, error) {
		panic("private-panic-token")
	}
	res, _, e = h.getPRInbox(context.Background(), nil, GetPRInboxInput{User: "octocat"})
	if e != nil || !res.IsError {
		t.Fatal("panic not recovered")
	}
	b, e := json.Marshal(res)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(b), "private-panic-token") {
		t.Fatal("panic payload leaked")
	}
}
func TestPRMCPConcurrentCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("mutation")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	cfg := config.Default()
	cfg.Token = "fixture"
	cfg.BaseURL = server.URL + "/"
	h := &handler{cfg: cfg}
	one := 1
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			a, out, e := h.getPRs(context.Background(), nil, GetPRsInput{Scope: "repos", Repos: []string{"acme/api"}, Since: "2026-08-24", Until: "2026-08-25", MaxRequests: &one})
			if e != nil || a.IsError || out.Cost.Consumed != 1 {
				t.Errorf("activity isolation: %+v %v", out, e)
			}
			b, inbox, e := h.getPRInbox(context.Background(), nil, GetPRInboxInput{Scope: "repos", Repos: []string{"acme/api"}, User: "octocat", MaxRequests: &one})
			if e != nil || b.IsError || inbox.Cost.Consumed != 1 {
				t.Errorf("inbox isolation: %+v %v", inbox, e)
			}
		})
	}
	wg.Wait()
}
