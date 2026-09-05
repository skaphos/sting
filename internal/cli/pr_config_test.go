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

// The startup warning in root.go promises this split, so pin it: an unreadable
// config leaves query and activity on defaults while the PR workflows refuse to
// run on defaults they cannot confirm.
func TestUnreadableConfigIsFatalOnlyForPRWorkflows(t *testing.T) {
	isolateHome(t)
	seedValidConfig(t)
	oldErr := configReadErr
	t.Cleanup(func() { configReadErr = oldErr })

	configReadErr = errors.New("parse failure")
	if _, err := loadConfig(); err != nil {
		t.Fatalf("commit and activity queries must continue from defaults: %v", err)
	}
	if _, err := loadPRConfig(); err == nil {
		t.Fatal("PR workflows accepted a config they could not read")
	}

	configReadErr = nil
	if _, err := loadPRConfig(); err != nil {
		t.Fatalf("readable config rejected: %v", err)
	}
}
