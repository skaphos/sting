// SPDX-License-Identifier: MIT
package cli

import (
	"context"
	"fmt"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/internal/commitclient"
	"github.com/skaphos/sting/internal/render"
	"github.com/skaphos/sting/model"
	"github.com/spf13/cobra"
)

var inboxCmd = &cobra.Command{Use: "inbox", Short: "Find open PRs authored by, assigned to, or requesting review from a user", Long: "Find current GitHub work without an age cutoff. Includes all drafts and the union of authored, assigned, and direct pending-review requests. Defaults to sting's authenticated user, 100 PRs and 500 requests; explicit zero disables a cap. Team-only requests are excluded. Unrestricted search is public-only; use explicit repositories or organization targets for private visibility. No provider actions or optional detail enrichment. Use sting prs for time-window activity.", Args: cobra.NoArgs, RunE: runInbox}

func registerInboxFlags(cmd *cobra.Command) {
	registerPRSharedFlags(cmd)
	cmd.Flags().String("user", "", "GitHub login (default: sting's authenticated user)")
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return fmt.Errorf("%w; use sting prs for date/state/role activity filters", err)
	})
}

var collectPRInbox = func(ctx context.Context, cfg config.Config, q model.PRInboxQuery) (model.PRInboxResult, error) {
	c, err := commitclient.NewPR(cfg)
	if err != nil {
		return model.PRInboxResult{}, err
	}
	return c.CollectPRInbox(ctx, q)
}

func runInbox(cmd *cobra.Command, _ []string) error {
	cfg, err := loadPRConfig()
	if err != nil {
		return err
	}
	req, err := prSharedRequest(cmd)
	if err != nil {
		return err
	}
	req.User, _ = cmd.Flags().GetString("user")
	q, err := cfg.ResolvePRInbox(req)
	if err != nil {
		return err
	}
	formatInput, _ := cmd.Flags().GetString("format")
	format, err := render.Parse(pick(formatInput, cfg.DefaultFormat))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), queryTimeout)
	defer cancel()
	result, collectErr := collectPRInbox(ctx, cfg, q)
	if result.SchemaVersion != "" {
		out, e := render.RenderPRInbox(result, format)
		if e != nil {
			return e
		}
		if _, e = fmt.Fprintln(cmd.OutOrStdout(), out); e != nil {
			return e
		}
	}
	return collectErr
}
