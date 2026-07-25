// SPDX-License-Identifier: MIT
package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/model"
)

func activityHandler() *handler {
	cfg := config.Default()
	cfg.Token = "test-token"
	return &handler{cfg: cfg}
}

// swapCollectActivity substitutes the collect function for the duration of a
// test, restoring it afterwards.
func swapCollectActivity(t *testing.T, fn func(context.Context, config.Config, model.ActivityQuery) (model.ActivityResult, error)) {
	t.Helper()
	prev := collectActivity
	collectActivity = fn
	t.Cleanup(func() { collectActivity = prev })
}

func TestGetRepoActivityMapsInput(t *testing.T) {
	var got model.ActivityQuery
	swapCollectActivity(t, func(_ context.Context, _ config.Config, q model.ActivityQuery) (model.ActivityResult, error) {
		got = q
		return model.ActivityResult{SchemaVersion: model.ActivitySchemaVersion, Repo: q.Repo}, nil
	})

	enrich := 5
	maxReq := 42
	includeDiffs := true
	in := GetRepoActivityInput{
		Repo:          "skaphos/sting",
		Ref:           "release/1.x",
		Since:         "2026-07-01",
		Until:         "2026-07-08",
		Author:        "octocat",
		IncludeDiffs:  &includeDiffs,
		MaxDiffBytes:  1234,
		EnrichCommits: &enrich,
		MaxRequests:   &maxReq,
	}

	res, out, err := activityHandler().getRepoActivity(context.Background(), nil, in)
	if err != nil {
		t.Fatalf("getRepoActivity: %v", err)
	}
	if res == nil || len(res.Content) == 0 {
		t.Fatal("no text content returned")
	}
	if out.SchemaVersion != model.ActivitySchemaVersion {
		t.Errorf("structured output missing the schema version: %+v", out)
	}

	if got.Repo != "skaphos/sting" {
		t.Errorf("Repo = %q", got.Repo)
	}
	if got.Ref != "release/1.x" {
		t.Errorf("Ref = %q", got.Ref)
	}
	if got.Author != "octocat" {
		t.Errorf("Author = %q", got.Author)
	}
	if !got.IncludeDiffs {
		t.Error("IncludeDiffs did not reach the query")
	}
	if got.MaxDiffBytes != 1234 {
		t.Errorf("MaxDiffBytes = %d, want 1234", got.MaxDiffBytes)
	}
	if got.EnrichCommits != 5 {
		t.Errorf("EnrichCommits = %d, want 5", got.EnrichCommits)
	}
	if got.MaxRequests != 42 {
		t.Errorf("MaxRequests = %d, want 42", got.MaxRequests)
	}
	if got.Provider != model.ProviderGitHub {
		t.Errorf("Provider = %q, want github", got.Provider)
	}
}

// TestGetRepoActivityDiffsAreOptInForMCP: diffs add output volume the agent did
// not ask for, so they default off regardless of server config.
func TestGetRepoActivityDiffsDefaultOff(t *testing.T) {
	var got model.ActivityQuery
	swapCollectActivity(t, func(_ context.Context, _ config.Config, q model.ActivityQuery) (model.ActivityResult, error) {
		got = q
		return model.ActivityResult{}, nil
	})

	h := activityHandler()
	h.cfg.IncludeDiffs = true // server config says yes; the tool still says no

	if _, _, err := h.getRepoActivity(context.Background(), nil, GetRepoActivityInput{Repo: "o/r"}); err != nil {
		t.Fatalf("getRepoActivity: %v", err)
	}
	if got.IncludeDiffs {
		t.Error("IncludeDiffs = true; MCP calls must opt in explicitly")
	}
}

// TestGetRepoActivityExplicitZeroMaxRequests: 0 means uncapped and must be
// distinguishable from an omitted field, which is why the input uses a pointer.
func TestGetRepoActivityExplicitZeroMaxRequests(t *testing.T) {
	var got model.ActivityQuery
	swapCollectActivity(t, func(_ context.Context, _ config.Config, q model.ActivityQuery) (model.ActivityResult, error) {
		got = q
		return model.ActivityResult{}, nil
	})

	zero := 0
	in := GetRepoActivityInput{Repo: "o/r", MaxRequests: &zero}
	if _, _, err := activityHandler().getRepoActivity(context.Background(), nil, in); err != nil {
		t.Fatalf("getRepoActivity: %v", err)
	}
	if got.MaxRequests != 0 {
		t.Errorf("MaxRequests = %d, want 0 (an explicit uncapped run)", got.MaxRequests)
	}

	// Omitted must fall back to the server default rather than 0.
	if _, _, err := activityHandler().getRepoActivity(context.Background(), nil, GetRepoActivityInput{Repo: "o/r"}); err != nil {
		t.Fatalf("getRepoActivity: %v", err)
	}
	if got.MaxRequests != model.DefaultMaxRequests {
		t.Errorf("MaxRequests = %d, want the default %d when omitted", got.MaxRequests, model.DefaultMaxRequests)
	}
}

// TestGetRepoActivityResolutionErrorIsTheErrorValue: the go-sdk marshals the
// structured output only when the error is nil, so returning the error here is
// what stops a failure emitting a schema-shaped, zero-value payload alongside
// the error text.
func TestGetRepoActivityResolutionErrorIsTheErrorValue(t *testing.T) {
	swapCollectActivity(t, func(context.Context, config.Config, model.ActivityQuery) (model.ActivityResult, error) {
		t.Fatal("collect must not be reached when resolution fails")
		return model.ActivityResult{}, nil
	})

	tests := []struct {
		name    string
		in      GetRepoActivityInput
		wantErr string
	}{
		{"missing repo", GetRepoActivityInput{}, "repo is required"},
		{"malformed repo", GetRepoActivityInput{Repo: "nope"}, "invalid github repo"},
		{"bad window", GetRepoActivityInput{Repo: "o/r", Window: "soon"}, "window:"},
		{"since after until", GetRepoActivityInput{
			Repo: "o/r", Since: "2026-07-10", Until: "2026-07-01",
		}, "is after until"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, out, err := activityHandler().getRepoActivity(context.Background(), nil, tt.in)
			if err == nil {
				t.Fatalf("expected an error; got res=%v out=%+v", res, out)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
			if res != nil {
				t.Error("a failure must not return a CallToolResult")
			}
			if out.SchemaVersion != "" {
				t.Errorf("a failure emitted a schema-shaped payload: %+v", out)
			}
		})
	}
}

func TestGetRepoActivityCollectErrorIsTheErrorValue(t *testing.T) {
	swapCollectActivity(t, func(context.Context, config.Config, model.ActivityQuery) (model.ActivityResult, error) {
		return model.ActivityResult{}, errString("repository unreadable: not found")
	})

	res, out, err := activityHandler().getRepoActivity(context.Background(), nil, GetRepoActivityInput{Repo: "o/r"})
	if err == nil {
		t.Fatal("expected the collect error to surface")
	}
	if !strings.Contains(err.Error(), "repository unreadable") {
		t.Errorf("error = %q", err)
	}
	if res != nil || out.SchemaVersion != "" {
		t.Error("a failure must not emit a structured payload")
	}
}

// TestGetRepoActivityPanicRecovered: the go-sdk dispatches handlers with no
// recovery of its own, so an unrecovered panic would take down the whole
// long-lived stdio server rather than failing one call.
func TestGetRepoActivityPanicRecovered(t *testing.T) {
	swapCollectActivity(t, func(context.Context, config.Config, model.ActivityQuery) (model.ActivityResult, error) {
		panic("boom in the collect chain")
	})

	res, out, err := activityHandler().getRepoActivity(context.Background(), nil, GetRepoActivityInput{Repo: "o/r"})
	if err != nil {
		t.Fatalf("a panic must be converted to a tool-level error result, got err = %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("expected an IsError tool result after a panic")
	}
	if len(res.Content) == 0 {
		t.Fatal("panic result has no content for the agent to read")
	}
	if out.SchemaVersion != "" {
		t.Errorf("panic result emitted a structured payload: %+v", out)
	}
}

// TestGetRepoActivityBudgetStopIsSuccess is the important one: a bounded result
// is a result. Returning an error would discard the evidence already gathered.
func TestGetRepoActivityBudgetStopIsSuccess(t *testing.T) {
	swapCollectActivity(t, func(_ context.Context, _ config.Config, q model.ActivityQuery) (model.ActivityResult, error) {
		return model.ActivityResult{
			SchemaVersion: model.ActivitySchemaVersion,
			Repo:          q.Repo,
			Count:         3,
			Commits:       make([]model.ActivityCommit, 3),
			Cost:          model.CostReport{Consumed: 2, Ceiling: 2},
			Disclosures: []model.Disclosure{{
				Kind: model.DisclosureBudgetBounded, Reason: "ceiling reached", NextAction: "raise it",
			}},
		}, nil
	})

	res, out, err := activityHandler().getRepoActivity(context.Background(), nil, GetRepoActivityInput{Repo: "o/r"})
	if err != nil {
		t.Fatalf("a budget stop must not be an error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatal("a budget stop must return a success result")
	}
	if out.Count != 3 {
		t.Errorf("Count = %d, want the 3 gathered commits preserved", out.Count)
	}
	if len(out.Disclosures) == 0 {
		t.Error("the budget-bounded disclosure was lost")
	}
	// The Markdown view must carry the disclosure too, or an agent reading only
	// the text will overstate completeness.
	text := firstText(res)
	if !strings.Contains(text, model.DisclosureBudgetBounded) {
		t.Errorf("Markdown view omits the budget disclosure:\n%s", text)
	}
}

func TestGetRepoActivityMarkdownAccompaniesStructured(t *testing.T) {
	swapCollectActivity(t, func(_ context.Context, _ config.Config, q model.ActivityQuery) (model.ActivityResult, error) {
		return model.ActivityResult{
			SchemaVersion: model.ActivitySchemaVersion,
			Repo:          q.Repo,
			Ref:           "main",
			Count:         1,
			Commits:       []model.ActivityCommit{{SHA: "abc1234", Message: "feat: thing"}},
		}, nil
	})

	res, out, err := activityHandler().getRepoActivity(context.Background(), nil, GetRepoActivityInput{Repo: "skaphos/sting"})
	if err != nil {
		t.Fatalf("getRepoActivity: %v", err)
	}
	text := firstText(res)
	if !strings.Contains(text, "skaphos/sting") {
		t.Errorf("Markdown does not name the repository:\n%s", text)
	}
	// The text is a view of the structured result, never a superset.
	if out.Count != 1 {
		t.Errorf("structured Count = %d, want 1", out.Count)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
