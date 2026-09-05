// SPDX-License-Identifier: MIT
package render_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/skaphos/sting/internal/render"
	"github.com/skaphos/sting/model"
)

func TestQueryDateMetadataJSONAndMarkdown(t *testing.T) {
	author := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	committer := author.Add(19 * 24 * time.Hour)
	r := model.Result{SchemaVersion: model.SchemaVersion, WindowDateBasis: model.WindowDateBasisMixed, Count: 1,
		Commits: []model.Commit{{SHA: "rebased", Date: author, CommitterDate: committer, WindowDateBasis: model.WindowDateBasisCommitter}}}
	out, err := render.Render(r, render.FormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip model.Result
	if err := json.Unmarshal([]byte(out), &roundTrip); err != nil {
		t.Fatal(err)
	}
	if roundTrip.Commits[0].Date != author || roundTrip.Commits[0].CommitterDate != committer || roundTrip.WindowDateBasis != model.WindowDateBasisMixed {
		t.Fatalf("dates or basis lost: %+v", roundTrip)
	}
	md := render.Markdown(r)
	for _, want := range []string{"Window date basis:", "repository listing: committer; open PRs: author", "author dates", "2026-07-01", "committed: 2026-07-20", "window: committer date"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in %s", want, md)
		}
	}
}

func TestEmptyIncompleteSearchRendersDisclosure(t *testing.T) {
	r := model.Result{Truncated: true, Disclosures: []model.Disclosure{
		{Kind: model.DisclosureSearchIncomplete, Reason: "Search timed out.", NextAction: "Narrow the window."},
	}}
	md := render.Markdown(r)
	for _, want := range []string{"truncated", "search-incomplete", "Search timed out.", "Narrow the window."} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in %s", want, md)
		}
	}
	if strings.Contains(md, "No commits found in this window") {
		t.Fatal("incomplete response rendered as verified absence")
	}
}
