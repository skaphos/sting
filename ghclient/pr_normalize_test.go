// SPDX-License-Identifier: MIT
package ghclient

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-github/v91/github"
)

func TestPRNormalizeAvailability(t *testing.T) {
	payload := prPayload(1)
	payload["additions"] = 0
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var raw github.PullRequest
	if err = json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	p, ok := fromPR(&raw, "acme/api")
	if !ok {
		t.Fatal("lost identity")
	}
	if p.State == nil || *p.State != "open" || p.Draft == nil || *p.Draft || p.Assignees == nil || len(p.Assignees) != 0 || p.Additions == nil || *p.Additions != 0 || p.Deletions != nil {
		t.Fatalf("availability: %+v", p)
	}
	if _, ok = fromPR(&github.PullRequest{}, "acme/api"); ok {
		t.Fatal("accepted missing identity")
	}
	issue := github.Issue{Number: new(2), HTMLURL: new("https://github.com/acme/api/pull/2"), State: new("closed"), RepositoryURL: new("https://api.github.com/repos/acme/api")}
	p, ok = fromPRIssue(&issue)
	if !ok || p.State != nil {
		t.Fatalf("guessed merge disposition: %+v", p)
	}
}

func TestPRDisclosuresAndErrorSanitization(t *testing.T) {
	c, err := New("", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	s, err := c.newPRSession(2)
	if err != nil {
		t.Fatal(err)
	}
	s.gap("history-incomplete", "missing history", "acme/api", 1)
	s.gap("history-incomplete", "missing history", "acme/api", 1)
	s.note("metadata-unavailable", "stats absent", "acme/api", 1)
	s.finish()
	if len(s.disclosures) != 2 || s.coverage.EvidenceComplete || !s.coverage.DiscoveryComplete {
		t.Fatalf("coverage: %+v", s.coverage)
	}
	secret := errors.New("https://token-secret@api.example.com/private")
	failure := s.stop(secret, "list PRs", "acme/api")
	if failure == nil || !errors.Is(failure, secret) || strings.Contains(failure.Error(), "token-secret") {
		t.Fatalf("unsafe error: %v", failure)
	}
	for _, d := range s.disclosures {
		if strings.Contains(d.Reason, "token-secret") {
			t.Fatal("secret disclosure")
		}
	}
}
