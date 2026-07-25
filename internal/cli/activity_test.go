// SPDX-License-Identifier: MIT
package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/model"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// activityTestServer serves the three endpoints an activity run touches, so the
// CLI can be exercised end to end without the network.
func activityTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "4990")
		p := r.URL.Path
		switch {
		case strings.Contains(p, "/compare/"):
			_, _ = w.Write([]byte(`{"status":"ahead","files":[
				{"filename":"a.go","status":"modified","additions":3,"deletions":1,"changes":4}
			]}`))
		case strings.HasSuffix(p, "/commits"):
			_, _ = w.Write([]byte(`[{
				"sha":"head111","html_url":"https://example.com/c/head111",
				"parents":[{"sha":"base000"}],
				"commit":{"message":"feat: a thing","author":{"name":"Octo","date":"2026-07-20T10:00:00Z"},
				"committer":{"name":"Octo","date":"2026-07-20T10:00:00Z"}}}]`))
		default:
			_, _ = w.Write([]byte(`{"default_branch":"main"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newActivityCmd builds a standalone activity command with the flag set
// registered, isolated from the global command tree and its viper state. The
// package-level viper is swapped for a fresh one and restored afterwards, so a
// test cannot leak configuration into its neighbours.
func newActivityCmd(t *testing.T, srvURL string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	isolateHome(t)

	prev := v
	t.Cleanup(func() { v = prev })
	v = viper.New()
	for key, val := range config.Defaults() {
		v.SetDefault(key, val)
	}
	v.Set("base_url", srvURL+"/")
	v.Set("token", "test-token")

	cmd, out, _ := newCmd()
	cmd.RunE = runActivity
	registerActivityFlags(cmd)
	return cmd, out
}

// runActivityCmd executes the command with args and returns its error.
func runActivityCmd(cmd *cobra.Command, args ...string) error {
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestActivityCommandRequiresRepo(t *testing.T) {
	srv := activityTestServer(t)
	cmd, _ := newActivityCmd(t, srv.URL)

	err := runActivityCmd(cmd)
	if err == nil {
		t.Fatal("expected an error when --repo is omitted")
	}
	if !strings.Contains(err.Error(), "repo is required") {
		t.Errorf("error = %v, want it to name the missing repo", err)
	}
}

func TestActivityCommandRejectsMalformedRepo(t *testing.T) {
	srv := activityTestServer(t)
	cmd, _ := newActivityCmd(t, srv.URL)

	err := runActivityCmd(cmd, "--repo", "not-a-repo")
	if err == nil {
		t.Fatal("expected an error for a malformed repo")
	}
	if !strings.Contains(err.Error(), "invalid github repo") {
		t.Errorf("error = %v, want an invalid-repo complaint", err)
	}
}

func TestActivityCommandRejectsGitLab(t *testing.T) {
	srv := activityTestServer(t)
	cmd, _ := newActivityCmd(t, srv.URL)

	err := runActivityCmd(cmd, "--repo", "group/project", "--provider", "gitlab")
	if err == nil {
		t.Fatal("expected GitLab to be rejected")
	}
	if !strings.Contains(err.Error(), "does not support repository activity") {
		t.Errorf("error = %v, want the GitHub-only rejection", err)
	}
}

func TestActivityCommandJSONOutput(t *testing.T) {
	srv := activityTestServer(t)
	cmd, out := newActivityCmd(t, srv.URL)

	if err := runActivityCmd(cmd, "--repo", "skaphos/sting", "--window", "7d", "--format", "json"); err != nil {
		t.Fatalf("runActivity: %v", err)
	}

	var res model.ActivityResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("output is not the JSON contract: %v\n%s", err, out.String())
	}
	if res.SchemaVersion != model.ActivitySchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", res.SchemaVersion, model.ActivitySchemaVersion)
	}
	if res.Repo != "skaphos/sting" {
		t.Errorf("Repo = %q", res.Repo)
	}
	if res.Count != 1 {
		t.Errorf("Count = %d, want 1", res.Count)
	}
	if res.Boundaries.BaseSHA != "base000" {
		t.Errorf("BaseSHA = %q, want base000", res.Boundaries.BaseSHA)
	}
}

func TestActivityCommandMarkdownOutput(t *testing.T) {
	srv := activityTestServer(t)
	cmd, out := newActivityCmd(t, srv.URL)

	if err := runActivityCmd(cmd, "--repo", "skaphos/sting", "--window", "7d"); err != nil {
		t.Fatalf("runActivity: %v", err)
	}
	got := out.String()
	for _, want := range []string{"# Activity:", "## Commits", "## Change set", "## Cost", "## Disclosures"} {
		if !strings.Contains(got, want) {
			t.Errorf("Markdown missing %q\n%s", want, got)
		}
	}
}

// TestActivityCommandSucceedsWithoutAuthor is the gap the feature exists to
// close: `sting query` needs an author, `sting activity` must not.
func TestActivityCommandSucceedsWithoutAuthor(t *testing.T) {
	srv := activityTestServer(t)
	cmd, _ := newActivityCmd(t, srv.URL)

	if err := runActivityCmd(cmd, "--repo", "skaphos/sting", "--window", "7d"); err != nil {
		t.Fatalf("activity must not require an author: %v", err)
	}
}

func TestActivityCommandFlagsReachTheRequest(t *testing.T) {
	srv := activityTestServer(t)
	cmd, out := newActivityCmd(t, srv.URL)

	err := runActivityCmd(cmd,
		"--repo", "skaphos/sting",
		"--ref", "release/1.x",
		"--since", "2026-07-01",
		"--until", "2026-07-08",
		"--author", "octocat",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("runActivity: %v", err)
	}

	var res model.ActivityResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res.Ref != "release/1.x" {
		t.Errorf("Ref = %q, want release/1.x", res.Ref)
	}
	if res.Since.Format("2006-01-02") != "2026-07-01" {
		t.Errorf("Since = %v, want 2026-07-01", res.Since)
	}
	if res.Until.Format("2006-01-02") != "2026-07-08" {
		t.Errorf("Until = %v, want 2026-07-08", res.Until)
	}
	// An author filter narrows the commits but not the change set, and the
	// result has to say so.
	found := false
	for _, d := range res.Disclosures {
		if d.Kind == model.DisclosureAuthorFilterNotApplied {
			found = true
		}
	}
	if !found {
		t.Error("author filter did not produce an author-filter-not-applied disclosure")
	}
}

func TestActivityCommandBadFormat(t *testing.T) {
	srv := activityTestServer(t)
	cmd, _ := newActivityCmd(t, srv.URL)

	if err := runActivityCmd(cmd, "--repo", "skaphos/sting", "--format", "xml"); err == nil {
		t.Error("expected an error for an unknown format")
	}
}

func TestActivityCommandBadWindow(t *testing.T) {
	srv := activityTestServer(t)
	cmd, _ := newActivityCmd(t, srv.URL)

	if err := runActivityCmd(cmd, "--repo", "skaphos/sting", "--window", "not-a-window"); err == nil {
		t.Error("expected an error for an unparseable window")
	}
}

func TestActivityCommandRegisteredOnRoot(t *testing.T) {
	var found bool
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "activity" {
			found = true
			if cmd.Flags().Lookup("repo") == nil {
				t.Error("activity command has no --repo flag")
			}
		}
	}
	if !found {
		t.Error("activity is not registered on the root command")
	}
}

// TestActivityCommandEstimateReportsWithoutGathering is FR-011 at the CLI: an
// estimate must report a projected cost and say plainly that it collected
// nothing, so a reader cannot mistake it for a result.
func TestActivityCommandEstimateReportsWithoutGathering(t *testing.T) {
	srv := activityTestServer(t)
	cmd, out := newActivityCmd(t, srv.URL)

	if err := runActivityCmd(cmd, "--repo", "skaphos/sting", "--window", "7d", "--estimate"); err != nil {
		t.Fatalf("estimate run: %v", err)
	}

	got := out.String()
	for _, want := range []string{"Estimated cost:", "provider requests", "No evidence was gathered"} {
		if !strings.Contains(got, want) {
			t.Errorf("estimate output missing %q\n%s", want, got)
		}
	}
	// An estimate is not a digest: it must not render commits or a change set.
	if strings.Contains(got, "## Commits") || strings.Contains(got, "## Change set") {
		t.Errorf("estimate output rendered gathered evidence\n%s", got)
	}
}

// TestActivityCommandEstimateJSONStillTheContract: JSON callers get the full
// shape, with the estimate in Cost and an empty commit list.
func TestActivityCommandEstimateJSON(t *testing.T) {
	srv := activityTestServer(t)
	cmd, out := newActivityCmd(t, srv.URL)

	err := runActivityCmd(cmd, "--repo", "skaphos/sting", "--estimate", "--format", "json")
	if err != nil {
		t.Fatalf("estimate run: %v", err)
	}

	var res model.ActivityResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("estimate JSON is not the contract: %v\n%s", err, out.String())
	}
	if res.Count != 0 {
		t.Errorf("Count = %d, want 0 for an estimate", res.Count)
	}
	if res.Cost.Estimated == 0 {
		t.Error("Cost.Estimated = 0; the estimate was not reported")
	}
}

// TestActivityCommandBudgetBoundedExitsZero is Constitution VI at the exit
// code: a bounded result is a result. Exiting non-zero would tell a script the
// query failed when it did not, encouraging it to discard usable evidence.
func TestActivityCommandBudgetBoundedExitsZero(t *testing.T) {
	srv := activityTestServer(t)
	cmd, out := newActivityCmd(t, srv.URL)

	// A ceiling of 1 is spent by the pre-flight check alone, so gathering stops
	// almost immediately.
	err := runActivityCmd(cmd,
		"--repo", "skaphos/sting", "--window", "90d",
		"--max-requests", "1", "--format", "json")
	if err != nil {
		t.Fatalf("a budget-bounded run must exit 0, got error: %v", err)
	}

	var res model.ActivityResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res.SchemaVersion != model.ActivitySchemaVersion {
		t.Error("a bounded result is not well-formed")
	}
	if len(res.Disclosures) == 0 {
		t.Error("a bounded run produced no disclosure explaining the bound")
	}
	if res.Cost.Ceiling != 1 {
		t.Errorf("Cost.Ceiling = %d, want 1", res.Cost.Ceiling)
	}
}

func TestActivityCommandRejectsNegativeMaxRequests(t *testing.T) {
	srv := activityTestServer(t)
	cmd, _ := newActivityCmd(t, srv.URL)

	err := runActivityCmd(cmd, "--repo", "skaphos/sting", "--max-requests", "-1")
	if err == nil {
		t.Fatal("expected a negative ceiling to be rejected")
	}
	if !strings.Contains(err.Error(), "max_requests must be >= 0") {
		t.Errorf("error = %v", err)
	}
}
