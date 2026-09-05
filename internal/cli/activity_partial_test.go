// SPDX-License-Identifier: MIT
package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skaphos/sting/model"
)

func TestActivityCLIPrintsPartialResultBeforeError(t *testing.T) {
	for _, format := range []string{"json", "markdown"} {
		t.Run(format, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "/compare/"):
					http.Error(w, "outage", 500)
				case strings.HasSuffix(r.URL.Path, "/commits"):
					fmt.Fprint(w, `[{"sha":"head111","parents":[{"sha":"base"}],"commit":{"message":"retained evidence"}}]`)
				default:
					fmt.Fprint(w, `{"resources":{"core":{"remaining":4999,"limit":5000}}}`)
				}
			}))
			defer srv.Close()
			cmd, out := newActivityCmd(t, srv.URL)
			cmd.SilenceUsage, cmd.SilenceErrors = true, true // match the production root
			err := runActivityCmd(cmd, "--repo", "a/b", "--ref", "main", "--format", format)
			if err == nil || !strings.Contains(out.String(), "retained evidence") {
				t.Fatalf("lost failure or evidence: err=%v output=%s", err, out)
			}
			if format == "json" {
				var result model.ActivityResult
				if err := json.Unmarshal(out.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Count != 1 || result.Cost.Consumed != 2 || !result.CommitsCollected || result.ChangeSetCollected {
					t.Fatalf("incorrect partial contract: %+v", result)
				}
			}
		})
	}
}

func TestActivityCLIEstimatePreservesQuotaDisclosure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"resources":{"core":{"remaining":0,"limit":5000,"reset":2000000000}}}`)
	}))
	defer srv.Close()
	cmd, out := newActivityCmd(t, srv.URL)
	if err := runActivityCmd(cmd, "--repo", "a/b", "--estimate", "--format", "markdown"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "quota-exhausted") || !strings.Contains(out.String(), "No evidence was gathered") {
		t.Fatalf("estimate lost its disclosure: %s", out)
	}
}
