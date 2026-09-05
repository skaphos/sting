// SPDX-License-Identifier: MIT
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/model"
)

func TestPRInboxCLIFailures(t *testing.T) {
	seedValidConfig(t)
	old := collectPRInbox
	t.Cleanup(func() { collectPRInbox = old })
	for _, report := range []bool{false, true} {
		collectPRInbox = func(context.Context, config.Config, model.PRInboxQuery) (model.PRInboxResult, error) {
			if !report {
				return model.PRInboxResult{}, errors.New("client setup failed")
			}
			return model.PRInboxResult{SchemaVersion: model.PRInboxSchemaVersion, PRs: []model.PRInboxEntry{}, Truncated: true, Disclosures: []model.PRDisclosure{{Kind: "identity-unresolved", Reason: "user lookup failed"}}}, errors.New("user lookup failed")
		}
		cmd, out, _ := newCmd()
		cmd.SetContext(context.Background())
		registerInboxFlags(cmd)
		if e := cmd.ParseFlags([]string{"--format", "json"}); e != nil {
			t.Fatal(e)
		}
		if e := runInbox(cmd, nil); e == nil {
			t.Fatal("failure swallowed")
		}
		if report {
			var got model.PRInboxResult
			if e := json.Unmarshal(out.Bytes(), &got); e != nil {
				t.Fatal(e)
			}
			if got.SchemaVersion == "" || !got.Truncated {
				t.Fatal("partial report lost")
			}
		} else if out.Len() != 0 {
			t.Fatal("fabricated setup report")
		}
	}
}
