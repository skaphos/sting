// SPDX-License-Identifier: MIT
package cli

import (
	"strings"
	"time"

	"github.com/skaphos/sting/internal/config"
	"github.com/skaphos/sting/internal/ghclient"
	"github.com/skaphos/sting/internal/render"
	"github.com/spf13/cobra"
)

// registerQueryFlags attaches the per-query flags to cmd. They are local flags
// (not bound to viper) because they are request inputs that override the
// resolved config defaults for a single invocation.
func registerQueryFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.String("author", "", "GitHub username whose commits to fetch (required)")
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
	scope, _ := f.GetString("scope")
	repos, _ := f.GetStringSlice("repos")
	org, _ := f.GetString("org")
	format, _ := f.GetString("format")

	req := config.Request{
		Author: author,
		Since:  since,
		Until:  until,
		Window: window,
		Scope:  scope,
		Repos:  repos,
		Org:    org,
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

	client, err := ghclient.New(cfg.Token, cfg.BaseURL, cfg.PerPage)
	if err != nil {
		return err
	}

	result, err := client.Collect(cmd.Context(), q)
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
