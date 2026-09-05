// SPDX-License-Identifier: MIT
package commitclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/skaphos/sting/config"
)

func TestPRFactoryDedicatedCredential(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "ambient-secret")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer dedicated" {
			t.Error("wrong credential")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total_count":0,"items":[]}`))
	}))
	defer server.Close()
	cfg := config.Default()
	cfg.Token = "dedicated"
	cfg.BaseURL = server.URL + "/"
	client, err := NewPR(cfg)
	if err != nil {
		t.Fatal(err)
	}
	q, err := cfg.ResolvePRs(config.PRRequest{Author: "octocat", TimeBasis: "created"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.CollectPRs(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if result.Cost.Consumed != 1 {
		t.Fatal("unmetered factory")
	}
}

func TestPRFactorySanitizesSetupFailure(t *testing.T) {
	cfg := config.Default()
	cfg.Token = "dedicated-secret"
	cfg.BaseURL = "https://user:private-password@example.com:invalid/"
	_, err := NewPR(cfg)
	if err == nil {
		t.Fatal("invalid base URL accepted")
	}
	if strings.Contains(err.Error(), "private-password") || strings.Contains(err.Error(), "dedicated-secret") {
		t.Fatal("client setup exposed a credential")
	}
}
