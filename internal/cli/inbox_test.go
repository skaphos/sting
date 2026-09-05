// SPDX-License-Identifier: MIT
package cli

import (
	"context"
	"testing"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/model"
)

func TestPRInboxCLIConfig(t *testing.T) {
	seedValidConfig(t)
	v.Set("default_window", "invalid")
	v.Set("provider", "gitlab")
	old := collectPRInbox
	t.Cleanup(func() { collectPRInbox = old })
	collectPRInbox = func(_ context.Context, _ config.Config, q model.PRInboxQuery) (model.PRInboxResult, error) {
		if q.User != "octocat" || q.Provider != model.ProviderGitHub || q.Draft != model.PRDraftExclude {
			t.Errorf("query: %+v", q)
		}
		return model.PRInboxResult{SchemaVersion: model.PRInboxSchemaVersion, Query: q, PRs: []model.PRInboxEntry{}}, nil
	}
	cmd, _, _ := newCmd()
	cmd.SetContext(context.Background())
	registerInboxFlags(cmd)
	if e := cmd.ParseFlags([]string{"--user", "octocat", "--draft=false"}); e != nil {
		t.Fatal(e)
	}
	if e := runInbox(cmd, nil); e != nil {
		t.Fatal(e)
	}
}
func TestPRInboxCLIRejectsActivity(t *testing.T) {
	for _, flag := range []string{"--window", "--since", "--state", "--role", "--author", "--time-basis"} {
		cmd, _, _ := newCmd()
		registerInboxFlags(cmd)
		if e := cmd.ParseFlags([]string{flag, "unused"}); e == nil {
			t.Errorf("accepted %s", flag)
		}
	}
}
