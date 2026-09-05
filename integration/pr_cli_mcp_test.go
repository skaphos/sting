// SPDX-License-Identifier: MIT
package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPRCLIAndMCP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "sting.exe")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/sting")
	build.Dir = ".."
	if out, e := build.CombinedOutput(); e != nil {
		t.Fatalf("build: %v %s", e, out)
	}
	cases := []struct {
		name, workflow, scope, mode string
		cap                         int
		wantError                   bool
	}{
		{"search lifecycle", "prs", "search", "lifecycle", 500, false},
		{"repo lifecycle", "prs", "repos", "lifecycle", 500, false},
		{"org lifecycle", "prs", "org", "lifecycle", 500, false},
		{"history budget", "prs", "repos", "lifecycle", 1, false},
		{"provider failure", "prs", "repos", "failure", 500, true},
		{"search ceiling", "prs", "search", "capped", 500, false},
		{"old inbox", "inbox", "repos", "inbox", 500, false},
		{"org inbox", "inbox", "org", "inbox", 500, false},
		{"search inbox", "inbox", "search", "inbox", 500, false},
		{"inbox reconciliation", "inbox", "search", "changed", 500, false},
		{"interrupted inbox verification", "inbox", "search", "inbox", 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); prIntegrationProvider(t, tc.mode, w, r) }))
			defer server.Close()
			args := []string{tc.workflow, "--scope", tc.scope, "--max-requests", fmt.Sprint(tc.cap), "--format", "json"}
			input := map[string]any{"scope": tc.scope, "max_requests": tc.cap}
			if tc.scope == "repos" {
				args = append(args, "--repos", "acme/api")
				input["repos"] = []string{"acme/api"}
			}
			if tc.scope == "org" {
				args = append(args, "--org", "acme")
				input["org"] = "acme"
			}
			tool := "get_prs"
			if tc.workflow == "prs" {
				args = append(args, "--author", "octocat", "--since", "2026-08-18", "--until", "2026-08-25T13:00:00Z")
				input["author"] = "octocat"
				input["since"] = "2026-08-18"
				input["until"] = "2026-08-25T13:00:00Z"
			} else {
				args = append(args, "--user", "octocat")
				input["user"] = "octocat"
				tool = "get_pr_inbox"
			}
			cmd := stingCommand(t, ctx, binary, server.URL, args...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			e := cmd.Run()
			if (e != nil) != tc.wantError {
				t.Fatalf("CLI exit: %v %s", e, &stderr)
			}
			var cli map[string]any
			if e = json.Unmarshal(stdout.Bytes(), &cli); e != nil {
				t.Fatalf("CLI JSON: %v %s", e, &stdout)
			}
			cliCalls := calls.Load()
			if int64(cli["cost"].(map[string]any)["consumed"].(float64)) != cliCalls || cliCalls > int64(tc.cap) {
				t.Fatalf("HTTP cost differs: %v calls=%d", cli["cost"], cliCalls)
			}
			session := prConnect(t, ctx, stingCommand(t, ctx, binary, server.URL, "mcp"))
			res, e := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: input})
			if e != nil {
				t.Fatal(e)
			}
			if res.IsError != tc.wantError {
				t.Fatalf("MCP error status: %+v", res)
			}
			var agent map[string]any
			decodeStructured(t, res, &agent)
			delete(cli, "generated_at")
			delete(agent, "generated_at")
			if !reflect.DeepEqual(cli, agent) {
				a, _ := json.Marshal(cli)
				b, _ := json.Marshal(agent)
				t.Fatalf("CLI/MCP differ:\n%s\n%s", a, b)
			}
			if calls.Load() != cliCalls*2 {
				t.Fatal("MCP provider cost differs")
			}
			count := int(cli["count"].(float64))
			truncated := cli["truncated"].(bool)
			switch {
			case tc.mode == "failure" || tc.mode == "capped":
				if count != 0 || !truncated {
					t.Fatalf("failure/cap evidence: %v", cli)
				}
			case tc.mode == "changed":
				if count != 0 || !truncated {
					t.Fatalf("stale assignment survived: %v", cli)
				}
			case tc.cap == 1:
				if count < 1 || !truncated {
					t.Fatalf("budget discarded fetched matches: %v", cli)
				}
			case tc.workflow == "inbox":
				if count != 1 {
					t.Fatalf("inbox membership: %v", cli)
				}
				prs := cli["prs"].([]any)
				if len(prs[0].(map[string]any)["reasons"].([]any)) != 3 {
					t.Fatal("missing inclusion reasons")
				}
			default:
				if count != 3 || truncated {
					t.Fatalf("lifecycle membership: %v", cli)
				}
			}
		})
	}
	t.Run("MCP startup isolates unused window", func(t *testing.T) {
		var calls atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); prIntegrationProvider(t, "inbox", w, r) }))
		defer server.Close()
		cmd := stingCommand(t, ctx, binary, server.URL, "mcp")
		cmd.Env = append(cmd.Env, "STING_DEFAULT_WINDOW=invalid", "STING_PROVIDER=gitlab")
		session := prConnect(t, ctx, cmd)
		in := map[string]any{"scope": "repos", "repos": []string{"acme/api"}, "user": "octocat", "draft": false, "max_prs": 0}
		for range 2 {
			res, e := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_pr_inbox", Arguments: in})
			if e != nil || res.IsError {
				t.Fatalf("inbox blocked: %v %+v", e, res)
			}
			before := calls.Load()
			for _, name := range []string{"get_prs", "get_commits", "get_repo_activity"} {
				res, e = session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{"author": "octocat"}})
				if e != nil {
					t.Fatal(e)
				}
				if !res.IsError {
					t.Fatalf("invalid %s accepted", name)
				}
			}
			if calls.Load() != before {
				t.Fatal("validation made provider requests")
			}
		}
		for _, field := range []string{"window", "state", "role", "time_basis", "author"} {
			bad := map[string]any{"user": "octocat", field: "unused"}
			res, e := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_pr_inbox", Arguments: bad})
			if e != nil {
				t.Fatal(e)
			}
			if !res.IsError {
				t.Errorf("schema accepted %s", field)
			}
		}
	})
	t.Run("invalid startup config", func(t *testing.T) {
		for _, content := range []string{"default_window: [unterminated", "per_page: wrong-type", "per_page: 0", "max_requests: -1"} {
			cmd := stingCommand(t, ctx, binary, "http://127.0.0.1:1", "mcp")
			var configPath string
			for n, arg := range cmd.Args {
				if arg == "--config" {
					configPath = cmd.Args[n+1]
				}
			}
			if e := os.WriteFile(configPath, []byte(content), 0o600); e != nil {
				t.Fatal(e)
			}
			if _, e := cmd.CombinedOutput(); e == nil {
				t.Fatalf("accepted malformed/shared-invalid config: %s", content)
			}
		}
	})
}
func prConnect(t *testing.T, ctx context.Context, cmd *exec.Cmd) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "sting-pr-integration", Version: "1"}, nil)
	s, e := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.Close(); e != nil {
			t.Error(e)
		}
	})
	list, e := s.ListTools(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(list.Tools) != 4 {
		t.Fatalf("want four tools, got %d", len(list.Tools))
	}
	for _, tool := range list.Tools {
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("non-read-only tool %s", tool.Name)
		}
	}
	return s
}
func prIntegrationProvider(t *testing.T, mode string, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	path := strings.TrimPrefix(r.URL.Path, "/api/v3")
	if r.Method != http.MethodGet {
		t.Errorf("mutation: %s", r.Method)
		http.Error(w, "read-only", http.StatusMethodNotAllowed)
		return
	}
	write := func(v any) {
		if e := json.NewEncoder(w).Encode(v); e != nil {
			t.Error(e)
		}
	}
	payload := func(n int) map[string]any {
		return map[string]any{"number": n, "html_url": fmt.Sprintf("https://github.com/acme/api/pull/%d", n), "repository_url": "https://api.github.com/repos/acme/api", "title": "PR fixture", "state": "open", "draft": false, "user": map[string]any{"login": "octocat"}, "created_at": "2026-08-24T13:00:00Z", "updated_at": "2026-08-24T13:00:00Z", "assignees": []any{}, "requested_reviewers": []any{}, "requested_teams": []any{}, "labels": []any{}, "pull_request": map[string]any{}}
	}
	if path == "/orgs/acme/repos" {
		write([]any{map[string]any{"full_name": "acme/api", "private": true}})
		return
	}
	if path == "/user" {
		write(map[string]any{"login": "octocat"})
		return
	}
	if mode == "failure" {
		w.WriteHeader(500)
		write(map[string]any{"message": "fixture outage"})
		return
	}
	if strings.HasSuffix(path, "/events") {
		switch {
		case strings.Contains(path, "/issues/2/"):
			write([]any{map[string]any{"id": 21, "event": "merged", "created_at": "2026-08-24T14:00:00Z"}, map[string]any{"id": 22, "event": "closed", "created_at": "2026-08-24T14:00:00Z"}})
		case strings.Contains(path, "/issues/3/"):
			write([]any{map[string]any{"id": 31, "event": "closed", "created_at": "2026-08-24T15:00:00Z"}, map[string]any{"id": 32, "event": "reopened", "created_at": "2026-09-01T00:00:00Z"}})
		default:
			write([]any{})
		}
		return
	}
	if path != "/search/issues" && path != "/repos/acme/api/pulls" {
		t.Errorf("unexpected PR endpoint: %s", path)
		http.Error(w, "unexpected", 404)
		return
	}
	items := []any{}
	switch mode {
	case "lifecycle":
		for n := 1; n <= 4; n++ {
			p := payload(n)
			if n != 1 {
				p["created_at"] = "2015-01-01T00:00:00Z"
			}
			if n == 2 {
				p["state"] = "closed"
				p["merged_at"] = "2026-08-24T14:00:00Z"
				p["closed_at"] = "2026-08-24T14:00:00Z"
				p["pull_request"] = map[string]any{"merged_at": "2026-08-24T14:00:00Z"}
			}
			items = append(items, p)
		}
	case "inbox":
		p := payload(1)
		p["created_at"] = "2015-01-01T00:00:00Z"
		p["assignees"] = []any{map[string]any{"login": "octocat"}}
		p["requested_reviewers"] = []any{map[string]any{"login": "octocat"}}
		items = append(items, p)
	case "changed":
		p := payload(1)
		p["user"] = map[string]any{"login": "other"}
		if path == "/search/issues" {
			p["assignees"] = []any{map[string]any{"login": "octocat"}}
		}
		items = append(items, p)
	}
	if path == "/search/issues" {
		total := len(items)
		incomplete := mode == "capped"
		if incomplete {
			total = 1001
		}
		write(map[string]any{"total_count": total, "incomplete_results": incomplete, "items": items})
	} else {
		write(items)
	}
}
