// SPDX-License-Identifier: MIT
package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/internal/commitclient"
	"github.com/skaphos/sting/internal/render"
	"github.com/skaphos/sting/model"
	"github.com/spf13/cobra"
)

// loadPRConfig keeps shared decoding strict while leaving workflow validation
// to the selected resolver. An unused activity default must not block the inbox.
func loadPRConfig() (config.Config, error) {
	if configReadErr != nil {
		return config.Config{}, fmt.Errorf("read config: check file syntax and access")
	}
	var cfg config.Config
	if err := v.Unmarshal(&cfg); err != nil {
		return config.Config{}, fmt.Errorf("decode config: invalid field type")
	}
	return cfg, nil
}

var prsCmd = &cobra.Command{Use: "prs", Short: "Report PR opening, merging and closure actions during a window", Long: "Report GitHub PR activity in search/repos/org scope. Defaults: all current states, opened/merged/unmerged-closed actions, all drafts, configured seven-day window. Unrestricted search is public-only; explicit targets use credential visibility. Lifecycle history consumes max-requests. Optional detail/diff enrichment and GitLab are unsupported. Use sting inbox for current work without an age cutoff.", Args: cobra.NoArgs, RunE: runPRs}

func registerPRSharedFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.String("provider", "", "provider (github only)")
	f.String("scope", "", "search|repos|org (default from config)")
	f.StringSlice("repos", nil, "repositories as owner/name; comma-separated")
	f.String("org", "", "organization target")
	f.Bool("draft", false, "select drafts; --draft=false excludes drafts")
	f.Bool("no-draft", false, "exclude drafts; cannot combine with --draft")
	f.Int("max-prs", 0, "distinct PR cap (config default 100; explicit 0 disables)")
	f.Int("max-requests", 0, "request cap (config default 500; explicit 0 disables)")
	f.StringP("format", "o", "", "markdown|json (default from config)")
}
func registerPRFlags(cmd *cobra.Command) {
	registerPRSharedFlags(cmd)
	f := cmd.Flags()
	f.String("author", "", "PR author login (required for search)")
	f.String("role", "", "author only")
	f.String("since", "", "inclusive start, RFC3339 or date; overrides window")
	f.String("until", "", "inclusive end; defaults to now")
	f.String("window", "", "look-back window (config default seven days)")
	f.String("state", "all", "current state: open|merged|closed|all (closed excludes merged)")
	f.String("time-basis", "any", "created|updated|merged|closed|any; updated is latest-update matching only")
}
func prSharedRequest(cmd *cobra.Command) (config.PRInboxRequest, error) {
	f := cmd.Flags()
	r := config.PRInboxRequest{}
	r.Provider, _ = f.GetString("provider")
	r.Scope, _ = f.GetString("scope")
	r.Repos, _ = f.GetStringSlice("repos")
	r.Org, _ = f.GetString("org")
	if f.Changed("draft") && f.Changed("no-draft") {
		return r, fmt.Errorf("choose --draft or --no-draft, not both")
	}
	if f.Changed("draft") {
		d, _ := f.GetBool("draft")
		r.Draft = &d
	}
	if f.Changed("no-draft") {
		v, _ := f.GetBool("no-draft")
		if !v {
			return r, fmt.Errorf("--no-draft=false is unsupported; use --draft")
		}
		d := false
		r.Draft = &d
	}
	if f.Changed("max-prs") {
		n, _ := f.GetInt("max-prs")
		r.MaxPRs = &n
	}
	if f.Changed("max-requests") {
		n, _ := f.GetInt("max-requests")
		r.MaxRequests = &n
	}
	return r, nil
}

var collectPRActivity = func(ctx context.Context, cfg config.Config, q model.PRQuery) (model.PRResult, error) {
	c, err := commitclient.NewPR(cfg)
	if err != nil {
		return model.PRResult{}, err
	}
	return c.CollectPRs(ctx, q)
}

func runPRs(cmd *cobra.Command, _ []string) error {
	cfg, err := loadPRConfig()
	if err != nil {
		return err
	}
	base, err := prSharedRequest(cmd)
	if err != nil {
		return err
	}
	f := cmd.Flags()
	r := config.PRRequest{Provider: base.Provider, Scope: base.Scope, Repos: base.Repos, Org: base.Org, Draft: base.Draft, MaxPRs: base.MaxPRs, MaxRequests: base.MaxRequests}
	r.Author, _ = f.GetString("author")
	r.Role, _ = f.GetString("role")
	r.Since, _ = f.GetString("since")
	r.Until, _ = f.GetString("until")
	r.Window, _ = f.GetString("window")
	r.State, _ = f.GetString("state")
	r.TimeBasis, _ = f.GetString("time-basis")
	q, err := cfg.ResolvePRs(r, time.Now())
	if err != nil {
		return err
	}
	formatInput, _ := f.GetString("format")
	format, err := render.Parse(pick(formatInput, cfg.DefaultFormat))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), queryTimeout)
	defer cancel()
	result, collectErr := collectPRActivity(ctx, cfg, q)
	if result.SchemaVersion != "" {
		out, e := render.RenderPRs(result, format)
		if e != nil {
			return e
		}
		if _, e = fmt.Fprintln(cmd.OutOrStdout(), out); e != nil {
			return e
		}
	}
	return collectErr
}
