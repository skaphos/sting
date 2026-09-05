// SPDX-License-Identifier: MIT
package model_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/ghclient"
	"github.com/skaphos/sting/model"
)

func TestPRCollectedJSONAvailability(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("mutation")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"number":1,"html_url":"https://github.com/acme/api/pull/1","title":"known","user":{"login":"octocat"},"state":"open","draft":false,"created_at":"2026-08-24T00:00:00Z","updated_at":"2026-08-24T00:00:00Z","requested_reviewers":[],"additions":0}]`)
	}))
	defer srv.Close()
	cfg := config.Default()
	q, e := cfg.ResolvePRs(config.PRRequest{Scope: "repos", Repos: []string{"acme/api"}, TimeBasis: "created"}, time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC))
	if e != nil {
		t.Fatal(e)
	}
	client, e := ghclient.New("", srv.URL+"/", 100)
	if e != nil {
		t.Fatal(e)
	}
	result, e := client.CollectPRs(context.Background(), q)
	if e != nil {
		t.Fatal(e)
	}
	b, e := json.Marshal(result)
	if e != nil {
		t.Fatal(e)
	}
	var raw map[string]any
	if e = json.Unmarshal(b, &raw); e != nil {
		t.Fatal(e)
	}
	if raw["schema_version"] != model.PRSchemaVersion || result.Truncated || result.Count != 1 {
		t.Fatalf("optional absence changed membership: %s", b)
	}
	entries := raw["prs"].([]any)
	entry := entries[0].(map[string]any)
	p := entry["pr"].(map[string]any)
	if p["assignees"] != nil || len(p["requested_reviewers"].([]any)) != 0 || p["additions"] != float64(0) || p["deletions"] != nil {
		t.Fatalf("availability lost: %s", b)
	}
	if entry["matches_complete"] != true || len(entry["matches"].([]any)) != 1 {
		t.Fatalf("match evidence lost: %s", b)
	}
}
