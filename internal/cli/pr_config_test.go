// SPDX-License-Identifier: MIT
package cli

import (
	"errors"
	"testing"

	"github.com/skaphos/sting/config"
)

func TestPRApplicableConfig(t *testing.T) {
	isolateHome(t)
	seedValidConfig(t)
	oldErr := configReadErr
	t.Cleanup(func() { configReadErr = oldErr })
	configReadErr = nil
	v.Set("default_window", "invalid")
	v.Set("provider", "gitlab")
	cfg, err := loadPRConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cfg.ResolvePRInbox(config.PRInboxRequest{User: "octocat"}); err != nil {
		t.Fatal(err)
	}
	if _, err = loadConfig(); err == nil {
		t.Fatal("legacy validation relaxed")
	}
	configReadErr = errors.New("secret provider token")
	if _, err = loadPRConfig(); err == nil || err.Error() == configReadErr.Error() {
		t.Fatalf("unsafe config error: %v", err)
	}
	configReadErr = nil
	v.Set("max_prs", "not-an-integer")
	if _, err = loadPRConfig(); err == nil {
		t.Fatal("accepted invalid field type")
	}
}
