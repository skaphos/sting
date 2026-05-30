package render

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/skaphos/sting/internal/model"
)

func sampleResult() model.Result {
	return model.Result{
		Author: "mendedlink",
		Scope:  model.ScopeSearch,
		Since:  time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC),
		Until:  time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC),
		Count:  2,
		Commits: []model.Commit{
			{SHA: "abcdef1234567", Repo: "skaphos/sting", Date: time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC), Message: "Add MCP server\n\nbody"},
			{SHA: "1234567abcdef", Repo: "skaphos/sting", Date: time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC), Message: "Fix window parsing"},
		},
	}
}

func TestMarkdownGrouping(t *testing.T) {
	md := Markdown(sampleResult())
	if !strings.Contains(md, "# Commits by `mendedlink`") {
		t.Error("missing header")
	}
	if !strings.Contains(md, "## skaphos/sting") {
		t.Error("missing repo grouping")
	}
	// Newest commit first within the repo.
	fixIdx := strings.Index(md, "Fix window parsing")
	addIdx := strings.Index(md, "Add MCP server")
	if fixIdx == -1 || addIdx == -1 || fixIdx > addIdx {
		t.Errorf("commits not ordered newest-first (fix=%d add=%d)", fixIdx, addIdx)
	}
	// Only the summary line, not the body.
	if strings.Contains(md, "body") {
		t.Error("markdown should use commit summary, not full body")
	}
	// SHA shortened to 7 chars.
	if !strings.Contains(md, "`abcdef1`") {
		t.Error("SHA not shortened to 7 chars")
	}
}

func TestMarkdownEmpty(t *testing.T) {
	r := model.Result{Author: "x", Scope: model.ScopeSearch, Count: 0}
	md := Markdown(r)
	if !strings.Contains(md, "No commits found") {
		t.Error("empty result should note no commits")
	}
}

func TestRenderJSONRoundTrip(t *testing.T) {
	out, err := Render(sampleResult(), FormatJSON)
	if err != nil {
		t.Fatalf("Render json: %v", err)
	}
	var back model.Result
	if err := json.Unmarshal([]byte(out), &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Author != "mendedlink" || back.Count != 2 || len(back.Commits) != 2 {
		t.Errorf("round-trip mismatch: %+v", back)
	}
}

func TestParseFormat(t *testing.T) {
	for _, in := range []string{"", "markdown", "md", "MARKDOWN"} {
		if f, err := Parse(in); err != nil || f != FormatMarkdown {
			t.Errorf("Parse(%q) = %v, %v", in, f, err)
		}
	}
	if f, err := Parse("json"); err != nil || f != FormatJSON {
		t.Errorf("Parse(json) = %v, %v", f, err)
	}
	if _, err := Parse("xml"); err == nil {
		t.Error("Parse(xml): want error")
	}
}
