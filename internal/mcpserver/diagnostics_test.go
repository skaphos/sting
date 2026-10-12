// SPDX-License-Identifier: MIT
package mcpserver

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skaphos/sting/config"
)

// connect serves New(cfg, opts...) over an in-memory transport so tool calls
// take the same registration path a real runtime uses.
func connect(t *testing.T, cfg config.Config, opts ...Option) *mcp.ClientSession {
	t.Helper()
	server, err := New(cfg, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	serverT, clientT := mcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := server.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func diagnosticsConfig(t *testing.T) config.Config {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)
	cfg := config.Default()
	cfg.BaseURL = srv.URL + "/"
	cfg.Token = "fixture-secret"
	return cfg
}

var diagnosticCalls = []*mcp.CallToolParams{
	{Name: "get_commits", Arguments: map[string]any{"author": "alice", "scope": "repos", "repos": []string{"a/b"}}},
	{Name: "get_repo_activity", Arguments: map[string]any{"repo": "a/b"}},
	{Name: "get_prs", Arguments: map[string]any{"author": "alice", "scope": "repos", "repos": []string{"a/b"}}},
	{Name: "get_pr_inbox", Arguments: map[string]any{"user": "alice", "scope": "repos", "repos": []string{"a/b"}}},
}

// TestEveryToolCallReportsItsCredential drives all four tools and expects one
// report line per call. Tool outcomes are irrelevant here: the report is
// written when the client is built, before any provider data is interpreted.
func TestEveryToolCallReportsItsCredential(t *testing.T) {
	var diag bytes.Buffer
	cs := connect(t, diagnosticsConfig(t), WithDiagnostics(&diag))

	for i, call := range diagnosticCalls {
		if _, err := cs.CallTool(context.Background(), call); err != nil {
			t.Fatalf("%s: %v", call.Name, err)
		}
		lines := strings.Split(strings.TrimSuffix(diag.String(), "\n"), "\n")
		if len(lines) != i+1 {
			t.Fatalf("after %s: want %d report lines, got %q", call.Name, i+1, diag.String())
		}
		if want := "sting: credential: github 127.0.0.1: dedicated PAT"; lines[i] != want {
			t.Fatalf("%s report = %q, want %q", call.Name, lines[i], want)
		}
	}
	if strings.Contains(diag.String(), "fixture-secret") {
		t.Fatalf("report leaked the secret: %q", diag.String())
	}
}

// TestToolCallsAreSilentWithoutDiagnostics guards the default: no option (and
// a nil writer, which `sting mcp` passes when verbose is off) writes nothing.
func TestToolCallsAreSilentWithoutDiagnostics(t *testing.T) {
	cfg := diagnosticsConfig(t)
	for name, opts := range map[string][]Option{"no option": nil, "nil writer": {WithDiagnostics(nil)}} {
		t.Run(name, func(t *testing.T) {
			cs := connect(t, cfg, opts...)
			for _, call := range diagnosticCalls {
				if _, err := cs.CallTool(context.Background(), call); err != nil {
					t.Fatalf("%s: %v", call.Name, err)
				}
			}
		})
	}
}
