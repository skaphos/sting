// SPDX-License-Identifier: MIT
package cli

import (
	"context"
	"time"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/internal/commitclient"
	"github.com/skaphos/sting/internal/render"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// activityCmd implements `sting activity`: a repository-scoped, cost-bounded
// digest of what happened over a window, with no author required.
//
// The command is deliberately thin — flag parsing and request construction only
// — so the logic worth testing lives in config.ResolveActivity and
// ghclient.CollectActivity, both held to the standard coverage gate rather than
// this package's lower interactive floor.
var activityCmd = &cobra.Command{
	Use:   "activity",
	Short: "Summarize what happened in a GitHub repository over a time window",
	Long: "Prints a Markdown or JSON digest of a repository's activity: the window's " +
		"commits, the aggregate per-file change set derived from comparing the window's " +
		"boundary states, and what the query cost.\n\n" +
		"Unlike `sting query`, no author is required. The request count does not grow " +
		"per commit, so a busy window stays cheap. GitHub only.",
	Args: cobra.NoArgs,
	RunE: runActivity,
}

func registerActivityFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	// --provider exists so the GitHub-only limit is stated rather than implied:
	// asking for GitLab gets a specific rejection naming the capability, not a
	// confusing failure somewhere downstream.
	f.String("provider", "", "source control provider (github only; gitlab is rejected)")
	f.String("repo", "", "repository to examine as owner/name (required)")
	f.String("ref", "", "branch or tag to examine (default: the repository's default branch)")
	f.String("since", "", "window start (RFC3339 or YYYY-MM-DD); overrides --window")
	f.String("until", "", "window end (RFC3339 or YYYY-MM-DD); defaults to now")
	f.String("window", "", "look-back window when --since is unset (e.g. 7d, 2w, 48h)")
	f.String("author", "", "narrow the commit list to one author (the change set still covers all authors)")
	f.Bool("include-diffs", false, "include bounded patch text in the change set")
	f.Int("max-diff-bytes", 0, "patch byte cap when --include-diffs is set (0 = config default)")
	f.Int("enrich-commits", 0, "fetch per-commit file data for this many of the newest commits, enabling observed (not just inferred) path attribution; costs one request each")
	f.Int("max-requests", 0, "cap provider requests for this run (0 = uncapped; default from config)")
	f.Bool("estimate", false, "report the projected cost and remaining quota without gathering evidence")
	f.StringP("format", "o", "", "output format: markdown|json")
}

// applyActivityCostFlags maps the cost-control flags onto the request. Both are
// checked with Changed so an unset flag inherits config rather than overriding
// it — which matters for --max-requests, where an unset flag must not be read
// as an explicit request for an uncapped run.
func applyActivityCostFlags(f *pflag.FlagSet, req *config.ActivityRequest) {
	if f.Changed("enrich-commits") {
		enrich, _ := f.GetInt("enrich-commits")
		req.EnrichCommits = &enrich
	}
	if f.Changed("max-requests") {
		maxRequests, _ := f.GetInt("max-requests")
		req.MaxRequests = &maxRequests
	}
	if f.Changed("estimate") {
		estimate, _ := f.GetBool("estimate")
		req.EstimateOnly = estimate
	}
}

func runActivity(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	provider, _ := f.GetString("provider")
	repo, _ := f.GetString("repo")
	ref, _ := f.GetString("ref")
	since, _ := f.GetString("since")
	until, _ := f.GetString("until")
	window, _ := f.GetString("window")
	author, _ := f.GetString("author")
	format, _ := f.GetString("format")

	req := config.ActivityRequest{
		Provider: provider,
		Repo:     repo,
		Ref:      ref,
		Since:    since,
		Until:    until,
		Window:   window,
		Author:   author,
	}
	// Only pass flags the user actually set, so an unset flag inherits config
	// rather than overriding it with a zero value.
	if f.Changed("include-diffs") {
		includeDiffs, _ := f.GetBool("include-diffs")
		req.IncludeDiffs = &includeDiffs
	}
	if f.Changed("max-diff-bytes") {
		maxDiffBytes, _ := f.GetInt("max-diff-bytes")
		req.MaxDiffBytes = &maxDiffBytes
	}
	applyActivityCostFlags(f, &req)

	q, err := cfg.ResolveActivity(req, time.Now())
	if err != nil {
		return err
	}

	outFormat, err := render.Parse(pick(format, cfg.DefaultFormat))
	if err != nil {
		return err
	}

	client, err := commitclient.NewActivity(cfg, q)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), queryTimeout)
	defer cancel()

	// A bounded result is a result: CollectActivity returns evidence plus a
	// disclosure rather than an error when a ceiling or a quota stops it, so
	// the command exits 0 and the caller keeps the partial evidence.
	result, err := client.CollectActivity(ctx, q)
	if err != nil {
		return err
	}

	// An estimate is not a result, so it gets its own human-readable shape. JSON
	// callers still receive the full contract, whose Cost carries the estimate
	// and whose commit list is empty.
	if q.EstimateOnly && outFormat == render.FormatMarkdown {
		cmd.Print(render.ActivityEstimate(result.Cost))
		return nil
	}

	out, err := render.RenderActivity(result, outFormat)
	if err != nil {
		return err
	}
	cmd.Println(out)
	return nil
}
