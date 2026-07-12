// SPDX-License-Identifier: MIT
package config

import (
	"strings"
	"testing"
	"time"

	"github.com/skaphos/sting/model"
)

func TestParseWindow(t *testing.T) {
	day := 24 * time.Hour
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"7d", 7 * day, false},
		{"2w", 14 * day, false},
		{"48h", 48 * time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{" 1d ", day, false},
		{"", 0, true},
		{"-3d", 0, true},
		{"5x", 0, true},
		{"d", 0, true},
	}
	for _, tt := range tests {
		got, err := ParseWindow(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseWindow(%q): want error, got %v", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseWindow(%q): unexpected error %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseWindow(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// TestParseWindowOverflow covers the integer overflow finding: a huge "w" or
// "d" value multiplied by 7*day (or 24h) can wrap an int64 into a negative
// duration with err==nil, which then passes Validate and fails every query
// later with a misleading "since is after until" instead of a clear error
// here.
func TestParseWindowOverflow(t *testing.T) {
	tests := []string{"20000w", "9223372036854775807d"}
	for _, in := range tests {
		got, err := ParseWindow(in)
		if err == nil {
			t.Errorf("ParseWindow(%q): want error (overflow), got duration %v", in, got)
		}
		if got < 0 {
			t.Errorf("ParseWindow(%q): got negative duration %v with err=%v, want no silent wraparound", in, got, err)
		}
	}
}

func TestParseTime(t *testing.T) {
	if _, err := ParseTime("2026-05-01"); err != nil {
		t.Errorf("date form: unexpected error %v", err)
	}
	if _, err := ParseTime("2026-05-01T12:00:00Z"); err != nil {
		t.Errorf("RFC3339 form: unexpected error %v", err)
	}
	if _, err := ParseTime("nope"); err == nil {
		t.Error("invalid form: want error")
	}
}

func TestValidateProvider(t *testing.T) {
	cfg := Default()
	cfg.DefaultProvider = model.Provider("bogus")
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate: want error for invalid provider")
	}
	if !strings.Contains(err.Error(), "invalid provider") {
		t.Fatalf("Validate error = %q, want invalid provider", err)
	}
}

func TestValidateMaxDiffBytes(t *testing.T) {
	cfg := Default()
	cfg.MaxDiffBytes = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate: want error for negative max diff bytes")
	}
}

// TestValidateGitLabSearchScopeIncompatible covers the finding that provider
// gitlab with the built-in default_scope "search" is a permanently broken
// combination: gitlab does not support scope=search (see Resolve), so every
// bare query would fail. Validate must catch this pairing at load time.
func TestValidateGitLabSearchScopeIncompatible(t *testing.T) {
	cfg := Default()
	cfg.DefaultProvider = model.ProviderGitLab
	cfg.DefaultScope = model.ScopeSearch
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate: want error for provider=gitlab with default_scope=search")
	}
	if !strings.Contains(err.Error(), "does not support default_scope") {
		t.Errorf("Validate error = %q, want does-not-support message", err)
	}

	// A compatible scope is fine.
	cfg.DefaultScope = model.ScopeRepos
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: unexpected error for provider=gitlab, default_scope=repos: %v", err)
	}
}

func TestValidateMaxCommits(t *testing.T) {
	cfg := Default()
	cfg.MaxCommits = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate: want error for negative max commits")
	}
}
