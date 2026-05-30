// SPDX-License-Identifier: MIT
package cli

import (
	"context"
	"strings"
	"time"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/internal/commitclient"
	"github.com/skaphos/sting/internal/render"
	"github.com/spf13/cobra"
)

// queryTimeout bounds the GitHub API round-trips for a single CLI query so a
// stalled connection cannot hang the command indefinitely.
const queryTimeout = 2 * time.Minute

// registerQueryFlags attaches the per-query flags to cmd. They are local flags
// (not bound to viper) because they are request inputs that override the
// resolved config defaults for a single invocation.
func registerQueryFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.String("provider", "", "source control provider: github|gitlab")
	f.String("author", "", "provider username or author string whose commits to fetch (required)")
	f.String("since", "", "window start (RFC3339 or YYYY-MM-DD); overrides --window")
	f.String("until", "", "window end (RFC3339 or YYYY-MM-DD); defaults to now")
	f.String("window", "", "look-back window when --since is unset (e.g. 7d, 2w, 48h)")
	f.String("scope", "", "discovery scope: search|repos|org")
	f.StringSlice("repos", nil, "owner/repo targets (scope=repos)")
	f.String("org", "", "organization login (scope=org)")
	f.StringP("format", "o", "", "output format: markdown|json")
	f.Bool("stats", false, "include per-commit additions/deletions")
}

func runQuery(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()
	author, _ := f.GetString("author")
	// With no author and no subcommand, show help rather than an error so a bare
	// `sting` is friendly.
	if strings.TrimSpace(author) == "" {
		return cmd.Help()
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	since, _ := f.GetString("since")
	until, _ := f.GetString("until")
	window, _ := f.GetString("window")
	provider, _ := f.GetString("provider")
	scope, _ := f.GetString("scope")
	repos, _ := f.GetStringSlice("repos")
	org, _ := f.GetString("org")
	format, _ := f.GetString("format")

	req := config.Request{
		Provider: provider,
		Author:   author,
		Since:    since,
		Until:    until,
		Window:   window,
		Scope:    scope,
		Repos:    repos,
		Org:      org,
	}
	if f.Changed("stats") {
		stats, _ := f.GetBool("stats")
		req.IncludeStats = &stats
	}

	q, err := cfg.Resolve(req, time.Now())
	if err != nil {
		return err
	}

	outFormat, err := render.Parse(pick(format, cfg.DefaultFormat))
	if err != nil {
		return err
	}

	client, err := commitclient.New(cfg, q.Provider)
	if err != nil {
		return err
	}

	// Bound the GitHub round-trips so a hung connection cannot wedge the CLI.
	ctx, cancel := context.WithTimeout(cmd.Context(), queryTimeout)
	defer cancel()

	result, err := client.Collect(ctx, q)
	if err != nil {
		return err
	}

	out, err := render.Render(result, outFormat)
	if err != nil {
		return err
	}
	cmd.Println(out)
	return nil
}

func pick(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}
