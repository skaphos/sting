// SPDX-License-Identifier: MIT
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/model"
)

func TestPRFlagsRejectBeforeCollection(t *testing.T) {
	for _, args := range [][]string{{"--provider", "gitlab", "--author", "octocat"}, {"--role", "reviewer", "--author", "octocat"}, {"--draft", "--no-draft", "--author", "octocat"}, {"--no-draft=false", "--author", "octocat"}, {"--format", "invalid", "--author", "octocat"}} {
		t.Run(args[0], func(t *testing.T) {
			seedValidConfig(t)
			cmd, _, _ := newCmd()
			cmd.SetContext(context.Background())
			registerPRFlags(cmd)
			if err := cmd.ParseFlags(args); err != nil {
				t.Fatal(err)
			}
			if err := runPRs(cmd, nil); err == nil {
				t.Fatal("accepted invalid query")
			}
		})
	}
}

func TestPRCLIEmitsPartialJSON(t *testing.T) {
	seedValidConfig(t)
	original := collectPRActivity
	t.Cleanup(func() { collectPRActivity = original })
	collectPRActivity = func(_ context.Context, _ config.Config, q model.PRQuery) (model.PRResult, error) {
		return model.PRResult{SchemaVersion: model.PRSchemaVersion, Query: q, PRs: []model.PRActivityEntry{}, Truncated: true, Disclosures: []model.PRDisclosure{{Kind: "provider-error", Reason: "fixture failure"}}}, errors.New("fixture failure")
	}
	cmd, out, _ := newCmd()
	cmd.SetContext(context.Background())
	registerPRFlags(cmd)
	if err := cmd.ParseFlags([]string{"--author", "octocat", "--format", "json", "--draft=false", "--max-prs", "0", "--max-requests", "0"}); err != nil {
		t.Fatal(err)
	}
	if err := runPRs(cmd, nil); err == nil {
		t.Fatal("provider failure became success")
	}
	var result model.PRResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion == "" || result.Query.Draft != "exclude" || result.Query.MaxPRs != 0 || result.Query.MaxRequests != 0 || !result.Truncated {
		t.Fatalf("bad partial: %+v", result)
	}
}
