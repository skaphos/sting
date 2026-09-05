// SPDX-License-Identifier: MIT
package commitclient

import (
	"context"
	"fmt"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/ghclient"
	"github.com/skaphos/sting/model"
)

// PRClient is the implemented PR collection surface used by application adapters.
type PRClient interface {
	CollectPRs(context.Context, model.PRQuery) (model.PRResult, error)
	CollectPRInbox(context.Context, model.PRInboxQuery) (model.PRInboxResult, error)
}

// NewPR selects sting's dedicated GitHub credential and preserves the API host.
// Each public collection call establishes its own query-authoritative budget.
func NewPR(cfg config.Config) (PRClient, error) {
	client, err := ghclient.New(resolveGitHubToken(cfg), cfg.BaseURL, cfg.PerPage)
	if err != nil {
		return nil, fmt.Errorf("configure GitHub PR client: check base_url syntax")
	}
	return client, nil
}
