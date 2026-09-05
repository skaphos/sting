// SPDX-License-Identifier: MIT
package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skaphos/sting/model"
)

// These tests cross the executable boundary: Cobra/config/provider/rendering
// for CLI calls and JSON-RPC initialization/schema validation/stdio for MCP.
// Only the provider is simulated; no credentials or external API are needed.
func TestCLIAndMCP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "sting.exe")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/sting")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build sting: %v\n%s", err, out)
	}
	srv := httptest.NewServer(http.HandlerFunc(providerFixture))
	defer srv.Close()

	t.Run("CLI incomplete search", func(t *testing.T) {
		cmd := stingCommand(t, ctx, binary, srv.URL, "query", "--author", "octocat",
			"--scope", "search", "--since", "2026-07-18", "--until", "2026-07-25", "--format", "json")
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("CLI query: %v\n%s", err, &stderr)
		}
		var result model.Result
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatalf("invalid CLI JSON: %v\n%s", err, &stdout)
		}
		assertSearchEvidence(t, result)
	})

	t.Run("CLI partial activity exits nonzero", func(t *testing.T) {
		cmd := stingCommand(t, ctx, binary, srv.URL, "activity", "--repo", "a/b",
			"--ref", "main", "--since", "2026-07-18", "--until", "2026-07-25", "--format", "json")
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		var exitErr *exec.ExitError
		if err := cmd.Run(); !errors.As(err, &exitErr) {
			t.Fatalf("want nonzero process exit, got %v", err)
		}
		var result model.ActivityResult
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatalf("partial output is not JSON: %v\n%s", err, &stdout)
		}
		assertActivityEvidence(t, result)
		if !strings.Contains(stderr.String(), "compare") {
			t.Fatalf("missing attributable stderr error: %s", &stderr)
		}
	})

	t.Run("MCP stdio round trip", func(t *testing.T) {
		cmd := stingCommand(t, ctx, binary, srv.URL, "mcp")
		client := mcp.NewClient(&mcp.Implementation{Name: "sting-integration", Version: "1"}, nil)
		session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
		if err != nil {
			t.Fatalf("MCP initialization: %v", err)
		}
		defer func() {
			if err := session.Close(); err != nil {
				t.Errorf("close MCP: %v", err)
			}
		}()
		list, err := session.ListTools(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		names := map[string]bool{}
		for _, tool := range list.Tools {
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
				t.Errorf("tool %s is not read-only", tool.Name)
			}
			names[tool.Name] = true
		}
		if len(names) != 2 || !names["get_commits"] || !names["get_repo_activity"] {
			t.Fatalf("unexpected tool registry: %v", names)
		}
		commits, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_commits",
			Arguments: map[string]any{"author": "octocat", "scope": "search", "since": "2026-07-18", "until": "2026-07-25"}})
		if err != nil {
			t.Fatal(err)
		}
		if commits.IsError {
			t.Fatalf("incomplete search became a tool error: %+v", commits)
		}
		var searchResult model.Result
		decodeStructured(t, commits, &searchResult)
		assertSearchEvidence(t, searchResult)

		activity, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_repo_activity",
			Arguments: map[string]any{"repo": "a/b", "ref": "main", "since": "2026-07-18", "until": "2026-07-25"}})
		if err != nil {
			t.Fatal(err)
		}
		if !activity.IsError {
			t.Fatal("comparison failure lost tool error status")
		}
		var activityResult model.ActivityResult
		decodeStructured(t, activity, &activityResult)
		assertActivityEvidence(t, activityResult)
		if len(activity.Content) == 0 {
			t.Fatal("partial activity lost text content")
		}
	})
}

func stingCommand(t *testing.T, ctx context.Context, binary, baseURL string, args ...string) *exec.Cmd {
	t.Helper()
	home := t.TempDir()
	configPath := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	flags := []string{"--config", configPath, "--token", "fixture-token", "--base-url", baseURL + "/"}
	cmd := exec.CommandContext(ctx, binary, append(flags, args...)...)
	cmd.Dir = home
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		key = strings.ToUpper(key)
		if strings.HasPrefix(key, "STING_") || key == "HOME" || key == "USERPROFILE" || key == "XDG_CONFIG_HOME" ||
			key == "GITHUB_TOKEN" || key == "GITLAB_TOKEN" || key == "GH_TOKEN" {
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	cmd.Env = append(cmd.Env, "HOME="+home, "USERPROFILE="+home, "XDG_CONFIG_HOME="+home)
	return cmd
}

func providerFixture(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		http.Error(w, "read-only fixture", http.StatusMethodNotAllowed)
		return
	}
	const commit = `{"sha":"head","repository":{"full_name":"a/b"},"parents":[{"sha":"base"}],"commit":{"message":"retained evidence","author":{"name":"Octo","date":"2026-07-20T00:00:00Z"},"committer":{"date":"2026-07-21T00:00:00Z"}}}`
	switch {
	case strings.HasSuffix(r.URL.Path, "/rate_limit"):
		fmt.Fprint(w, `{"resources":{"core":{"remaining":4999,"limit":5000}}}`)
	case strings.Contains(r.URL.Path, "/search/commits"):
		fmt.Fprintf(w, `{"total_count":1,"incomplete_results":true,"items":[%s]}`, commit)
	case strings.Contains(r.URL.Path, "/compare/"):
		http.Error(w, "comparison outage", http.StatusInternalServerError)
	case strings.HasSuffix(r.URL.Path, "/repos/a/b/commits"):
		fmt.Fprintf(w, "[%s]", commit)
	default:
		http.Error(w, "unexpected endpoint", http.StatusNotFound)
	}
}

func decodeStructured(t *testing.T, result *mcp.CallToolResult, dst any) {
	t.Helper()
	if result.StructuredContent == nil {
		t.Fatal("missing structured MCP evidence")
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		t.Fatal(err)
	}
}

func assertSearchEvidence(t *testing.T, result model.Result) {
	t.Helper()
	if result.SchemaVersion != model.SchemaVersion || result.Count != 1 || len(result.Commits) != 1 ||
		!result.Truncated || result.WindowDateBasis != model.WindowDateBasisAuthor {
		t.Fatalf("incorrect search evidence: %+v", result)
	}
	if result.Commits[0].CommitterDate.IsZero() || result.Commits[0].Date.Equal(result.Commits[0].CommitterDate) {
		t.Fatal("lost distinct commit timestamps")
	}
	if len(result.Disclosures) != 1 || result.Disclosures[0].Kind != model.DisclosureSearchIncomplete {
		t.Fatalf("lost search disclosure: %+v", result.Disclosures)
	}
}

func assertActivityEvidence(t *testing.T, result model.ActivityResult) {
	t.Helper()
	if result.SchemaVersion != model.ActivitySchemaVersion || result.Count != 1 || len(result.Commits) != 1 ||
		!result.CommitsCollected || result.ChangeSetCollected || result.Cost.Consumed != 2 {
		t.Fatalf("lost partial activity: %+v", result)
	}
	if len(result.Disclosures) != 1 || result.Disclosures[0].Kind != model.DisclosureCollectionFailed {
		t.Fatalf("lost failure disclosure: %+v", result.Disclosures)
	}
}
