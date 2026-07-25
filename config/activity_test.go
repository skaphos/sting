// SPDX-License-Identifier: MIT
package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/skaphos/sting/config"
	"github.com/skaphos/sting/model"
)

func fixedNow() time.Time {
	return time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
}

func intPtr(i int) *int    { return &i }
func boolPtr(b bool) *bool { return &b }

func TestResolveActivity(t *testing.T) {
	t.Parallel()
	now := fixedNow()
	base := config.Default()

	tests := []struct {
		name    string
		cfg     config.Config
		req     config.ActivityRequest
		wantErr string // substring; empty means success expected
		check   func(t *testing.T, q model.ActivityQuery)
	}{
		{
			name: "window form",
			cfg:  base,
			req:  config.ActivityRequest{Repo: "skaphos/sting", Window: "7d"},
			check: func(t *testing.T, q model.ActivityQuery) {
				if got := q.Until.Sub(q.Since); got != 7*24*time.Hour {
					t.Errorf("window = %v, want 168h", got)
				}
				if !q.Until.Equal(now) {
					t.Errorf("Until = %v, want %v", q.Until, now)
				}
			},
		},
		{
			name: "since and until form",
			cfg:  base,
			req: config.ActivityRequest{
				Repo:  "skaphos/sting",
				Since: "2026-07-01",
				Until: "2026-07-08",
			},
			check: func(t *testing.T, q model.ActivityQuery) {
				wantSince := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
				wantUntil := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
				if !q.Since.Equal(wantSince) || !q.Until.Equal(wantUntil) {
					t.Errorf("window = %v..%v, want %v..%v", q.Since, q.Until, wantSince, wantUntil)
				}
			},
		},
		{
			name: "rfc3339 bounds",
			cfg:  base,
			req: config.ActivityRequest{
				Repo:  "skaphos/sting",
				Since: "2026-07-01T06:30:00Z",
				Until: "2026-07-02T18:45:00Z",
			},
			check: func(t *testing.T, q model.ActivityQuery) {
				if q.Since.Hour() != 6 || q.Since.Minute() != 30 {
					t.Errorf("Since = %v, want 06:30 precision preserved", q.Since)
				}
			},
		},
		{
			name: "defaults to configured window when none given",
			cfg:  base,
			req:  config.ActivityRequest{Repo: "skaphos/sting"},
			check: func(t *testing.T, q model.ActivityQuery) {
				if got := q.Until.Sub(q.Since); got != 7*24*time.Hour {
					t.Errorf("window = %v, want the configured default of 7d", got)
				}
			},
		},
		{
			name: "since only defaults until to now",
			cfg:  base,
			req:  config.ActivityRequest{Repo: "skaphos/sting", Since: "2026-07-20"},
			check: func(t *testing.T, q model.ActivityQuery) {
				if !q.Until.Equal(now) {
					t.Errorf("Until = %v, want now (%v)", q.Until, now)
				}
			},
		},
		{
			name:    "missing repo",
			cfg:     base,
			req:     config.ActivityRequest{Window: "7d"},
			wantErr: "repo is required (owner/name)",
		},
		{
			name:    "blank repo",
			cfg:     base,
			req:     config.ActivityRequest{Repo: "   ", Window: "7d"},
			wantErr: "repo is required (owner/name)",
		},
		{
			name:    "malformed repo without owner",
			cfg:     base,
			req:     config.ActivityRequest{Repo: "sting"},
			wantErr: `invalid github repo "sting"`,
		},
		{
			name:    "malformed repo with a space",
			cfg:     base,
			req:     config.ActivityRequest{Repo: "skaphos/sting extra"},
			wantErr: "invalid github repo",
		},
		{
			name:    "malformed repo with a qualifier character",
			cfg:     base,
			req:     config.ActivityRequest{Repo: "skaphos/sting:evil"},
			wantErr: "invalid github repo",
		},
		{
			name:    "since after until",
			cfg:     base,
			req:     config.ActivityRequest{Repo: "skaphos/sting", Since: "2026-07-10", Until: "2026-07-01"},
			wantErr: "is after until",
		},
		{
			name:    "negative max_requests",
			cfg:     base,
			req:     config.ActivityRequest{Repo: "skaphos/sting", MaxRequests: intPtr(-1)},
			wantErr: "max_requests must be >= 0, got -1",
		},
		{
			name:    "negative enrich_commits",
			cfg:     base,
			req:     config.ActivityRequest{Repo: "skaphos/sting", EnrichCommits: intPtr(-3)},
			wantErr: "enrich_commits must be >= 0, got -3",
		},
		{
			name:    "negative max_diff_bytes",
			cfg:     base,
			req:     config.ActivityRequest{Repo: "skaphos/sting", MaxDiffBytes: intPtr(-10)},
			wantErr: "max_diff_bytes must be >= 0, got -10",
		},
		{
			name:    "gitlab rejected",
			cfg:     base,
			req:     config.ActivityRequest{Provider: "gitlab", Repo: "group/project"},
			wantErr: `provider "gitlab" does not support repository activity (github only)`,
		},
		{
			name:    "gitlab rejected via configured default provider",
			cfg:     config.Config{DefaultProvider: model.ProviderGitLab, DefaultWindow: "7d"},
			req:     config.ActivityRequest{Repo: "group/project"},
			wantErr: "does not support repository activity (github only)",
		},
		{
			name:    "invalid provider",
			cfg:     base,
			req:     config.ActivityRequest{Provider: "bitbucket", Repo: "o/r"},
			wantErr: `invalid provider "bitbucket"`,
		},
		{
			name:    "invalid window",
			cfg:     base,
			req:     config.ActivityRequest{Repo: "o/r", Window: "seven days"},
			wantErr: "window:",
		},
		{
			name:    "invalid since",
			cfg:     base,
			req:     config.ActivityRequest{Repo: "o/r", Since: "yesterday"},
			wantErr: "since:",
		},
		{
			name:    "invalid until",
			cfg:     base,
			req:     config.ActivityRequest{Repo: "o/r", Until: "tomorrow"},
			wantErr: "until:",
		},
		{
			name: "max_requests defaults to the configured ceiling",
			cfg:  base,
			req:  config.ActivityRequest{Repo: "o/r"},
			check: func(t *testing.T, q model.ActivityQuery) {
				if q.MaxRequests != model.DefaultMaxRequests {
					t.Errorf("MaxRequests = %d, want %d", q.MaxRequests, model.DefaultMaxRequests)
				}
			},
		},
		{
			name: "explicit zero max_requests means uncapped",
			cfg:  base,
			req:  config.ActivityRequest{Repo: "o/r", MaxRequests: intPtr(0)},
			check: func(t *testing.T, q model.ActivityQuery) {
				if q.MaxRequests != 0 {
					t.Errorf("MaxRequests = %d, want 0 (explicit uncapped must survive)", q.MaxRequests)
				}
			},
		},
		{
			name: "request max_requests overrides config",
			cfg:  config.Config{DefaultWindow: "7d", MaxRequests: 500},
			req:  config.ActivityRequest{Repo: "o/r", MaxRequests: intPtr(25)},
			check: func(t *testing.T, q model.ActivityQuery) {
				if q.MaxRequests != 25 {
					t.Errorf("MaxRequests = %d, want 25 (flag must win over config)", q.MaxRequests)
				}
			},
		},
		{
			name: "include_diffs overrides config default",
			cfg:  config.Config{DefaultWindow: "7d", IncludeDiffs: true},
			req:  config.ActivityRequest{Repo: "o/r", IncludeDiffs: boolPtr(false)},
			check: func(t *testing.T, q model.ActivityQuery) {
				if q.IncludeDiffs {
					t.Error("IncludeDiffs = true, want the explicit false to win over config")
				}
			},
		},
		{
			name: "include_diffs inherits config when unset",
			cfg:  config.Config{DefaultWindow: "7d", IncludeDiffs: true},
			req:  config.ActivityRequest{Repo: "o/r"},
			check: func(t *testing.T, q model.ActivityQuery) {
				if !q.IncludeDiffs {
					t.Error("IncludeDiffs = false, want it inherited from config")
				}
			},
		},
		{
			name: "max_diff_bytes falls back to the model default when config is zero",
			cfg:  config.Config{DefaultWindow: "7d"},
			req:  config.ActivityRequest{Repo: "o/r"},
			check: func(t *testing.T, q model.ActivityQuery) {
				if q.MaxDiffBytes != model.DefaultMaxDiffBytes {
					t.Errorf("MaxDiffBytes = %d, want %d", q.MaxDiffBytes, model.DefaultMaxDiffBytes)
				}
			},
		},
		{
			name: "enrich_commits defaults to zero",
			cfg:  base,
			req:  config.ActivityRequest{Repo: "o/r"},
			check: func(t *testing.T, q model.ActivityQuery) {
				if q.EnrichCommits != 0 {
					t.Errorf("EnrichCommits = %d, want 0 (enrichment is opt-in)", q.EnrichCommits)
				}
			},
		},
		{
			name: "ref and author are trimmed and preserved",
			cfg:  base,
			req:  config.ActivityRequest{Repo: "o/r", Ref: "  release/1.x  ", Author: "  someone  "},
			check: func(t *testing.T, q model.ActivityQuery) {
				if q.Ref != "release/1.x" {
					t.Errorf("Ref = %q, want %q", q.Ref, "release/1.x")
				}
				if q.Author != "someone" {
					t.Errorf("Author = %q, want %q", q.Author, "someone")
				}
			},
		},
		{
			name: "estimate only is carried through",
			cfg:  base,
			req:  config.ActivityRequest{Repo: "o/r", EstimateOnly: true},
			check: func(t *testing.T, q model.ActivityQuery) {
				if !q.EstimateOnly {
					t.Error("EstimateOnly = false, want true")
				}
			},
		},
		{
			name: "provider defaults to github",
			cfg:  config.Config{DefaultWindow: "7d"},
			req:  config.ActivityRequest{Repo: "o/r"},
			check: func(t *testing.T, q model.ActivityQuery) {
				if q.Provider != model.ProviderGitHub {
					t.Errorf("Provider = %q, want github", q.Provider)
				}
			},
		},
		{
			name: "empty window equal bounds is not an error",
			cfg:  base,
			req:  config.ActivityRequest{Repo: "o/r", Since: "2026-07-01", Until: "2026-07-01"},
			check: func(t *testing.T, q model.ActivityQuery) {
				if !q.Since.Equal(q.Until) {
					t.Errorf("expected an empty window, got %v..%v", q.Since, q.Until)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			q, err := tt.cfg.ResolveActivity(tt.req, now)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ResolveActivity() = %+v, want error containing %q", q, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveActivity() error = %v", err)
			}
			if tt.check != nil {
				tt.check(t, q)
			}
		})
	}
}

// TestResolveActivityNormalizesToUTC is Principle III at the boundary: the
// window is normalized exactly once, here. If a non-UTC bound leaked downstream,
// the same instant could format two different ways in the result.
func TestResolveActivityNormalizesToUTC(t *testing.T) {
	t.Parallel()
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Skipf("timezone data unavailable: %v", err)
	}
	now := time.Date(2026, 7, 25, 21, 0, 0, 0, tokyo)

	q, err := config.Default().ResolveActivity(config.ActivityRequest{Repo: "o/r", Window: "1d"}, now)
	if err != nil {
		t.Fatalf("ResolveActivity: %v", err)
	}
	if q.Since.Location() != time.UTC || q.Until.Location() != time.UTC {
		t.Errorf("window not normalized to UTC: since=%v until=%v", q.Since.Location(), q.Until.Location())
	}
	// Normalization must change only the representation, never the instant.
	if !q.Until.Equal(now) {
		t.Errorf("Until = %v, want the same instant as %v", q.Until, now)
	}
}

// TestResolveActivityDeterministic is FR-023 at the resolution boundary:
// identical requests at a fixed now must normalize identically, or nothing
// downstream can be deterministic either.
func TestResolveActivityDeterministic(t *testing.T) {
	t.Parallel()
	now := fixedNow()
	req := config.ActivityRequest{
		Repo:          "skaphos/sting",
		Ref:           "main",
		Window:        "7d",
		Author:        "someone",
		EnrichCommits: intPtr(5),
		MaxRequests:   intPtr(100),
	}

	first, err := config.Default().ResolveActivity(req, now)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	for i := range 10 {
		got, err := config.Default().ResolveActivity(req, now)
		if err != nil {
			t.Fatalf("resolve %d: %v", i, err)
		}
		if got != first {
			t.Fatalf("resolve %d = %+v, want identical to %+v", i, got, first)
		}
	}
}

func TestConfigMaxRequestsDefaultAndValidation(t *testing.T) {
	t.Parallel()

	if got := config.Default().MaxRequests; got != model.DefaultMaxRequests {
		t.Errorf("Default().MaxRequests = %d, want %d", got, model.DefaultMaxRequests)
	}
	if got, ok := config.Defaults()["max_requests"]; !ok {
		t.Error("Defaults() is missing the max_requests key")
	} else if got != model.DefaultMaxRequests {
		t.Errorf("Defaults()[max_requests] = %v, want %d", got, model.DefaultMaxRequests)
	}

	cfg := config.Default()
	cfg.MaxRequests = -1
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "max_requests must be >= 0, got -1") {
		t.Errorf("Validate() error = %v, want a max_requests bound complaint", err)
	}

	cfg.MaxRequests = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() with an explicit uncapped 0 = %v, want nil", err)
	}
}
