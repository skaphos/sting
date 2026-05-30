package render_test

import (
	"fmt"
	"time"

	"github.com/skaphos/sting/internal/model"
	"github.com/skaphos/sting/internal/render"
)

// ExampleMarkdown renders a query result as a Markdown report grouped by
// repository, newest commit first.
func ExampleMarkdown() {
	result := model.Result{
		Author: "octocat",
		Scope:  model.ScopeSearch,
		Since:  time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC),
		Until:  time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC),
		Count:  1,
		Commits: []model.Commit{{
			SHA:     "abc1234def5678",
			Repo:    "skaphos/sting",
			Date:    time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC),
			Message: "Add example\n\nbody ignored in the summary line",
		}},
	}

	fmt.Print(render.Markdown(result))
	// Output:
	// # Commits by `octocat`
	//
	// - **Window:** 2026-05-22 → 2026-05-29
	// - **Scope:** search
	// - **Commits:** 1
	//
	// ## skaphos/sting
	//
	// - `abc1234` 2026-05-28 — Add example
}
