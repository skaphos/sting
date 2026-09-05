// SPDX-License-Identifier: MIT
package model

import (
	"encoding/json"
	"testing"
)

func TestPRMetadataAvailabilityJSON(t *testing.T) {
	zero := 0
	no := false
	p := PullRequest{Repo: "acme/api", Number: 1, Draft: &no, Additions: &zero, Assignees: []string{}, MissingFields: []string{"requested_reviewers"}}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"state", "draft", "created_at", "updated_at", "merged_at", "closed_at", "assignees", "requested_reviewers", "requested_teams", "labels", "base_ref", "head_ref", "milestone", "additions", "deletions", "changed_files", "missing_fields"} {
		if _, ok := got[field]; !ok {
			t.Errorf("missing %s", field)
		}
	}
	if got["requested_reviewers"] != nil || len(got["assignees"].([]any)) != 0 || got["additions"] != float64(0) || got["draft"] != false {
		t.Fatalf("availability lost: %s", b)
	}
}
func TestPRIndependentContracts(t *testing.T) {
	if PRSchemaVersion == PRInboxSchemaVersion || PRSchemaVersion == SchemaVersion || PRInboxSchemaVersion == ActivitySchemaVersion {
		t.Fatal("schema collision")
	}
	for _, value := range []any{PRResult{SchemaVersion: PRSchemaVersion, PRs: []PRActivityEntry{}, Disclosures: []PRDisclosure{}}, PRInboxResult{SchemaVersion: PRInboxSchemaVersion, PRs: []PRInboxEntry{}, Disclosures: []PRDisclosure{}}} {
		b, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err = json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		if len(got["prs"].([]any)) != int(got["count"].(float64)) {
			t.Fatal("count mismatch")
		}
	}
	b, err := json.Marshal(PRInboxQuery{})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"since", "until", "window", "time_basis", "author", "role"} {
		if _, ok := got[key]; ok {
			t.Errorf("inbox leaked %s", key)
		}
	}
}
