// SPDX-License-Identifier: MIT
package config

import (
	"strings"
	"testing"
	"time"
)

func TestResolversShareUTCWindowPolicy(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 30, 0, 0, time.FixedZone("offset", -5*60*60))
	for _, tc := range []struct {
		name, since, until, window, wantSince, wantUntil, wantErr string
	}{
		{name: "default", wantSince: "2026-07-18T17:30:00Z", wantUntil: "2026-07-25T17:30:00Z"},
		{name: "window", window: "48h", wantSince: "2026-07-23T17:30:00Z", wantUntil: "2026-07-25T17:30:00Z"},
		{name: "explicit offsets override window", since: "2026-07-18T02:00:00+02:00", until: "2026-07-25T01:00:00-05:00", window: "invalid", wantSince: "2026-07-18T00:00:00Z", wantUntil: "2026-07-25T06:00:00Z"},
		{name: "date only", since: "2026-07-18", until: "2026-07-25", wantSince: "2026-07-18T00:00:00Z", wantUntil: "2026-07-25T00:00:00Z"},
		{name: "equal bounds", since: "2026-07-18T01:00:00+01:00", until: "2026-07-18T00:00:00Z", wantSince: "2026-07-18T00:00:00Z", wantUntil: "2026-07-18T00:00:00Z"},
		{name: "until first", since: "bad", until: "bad", wantErr: "until:"},
		{name: "since invalid", since: "bad", wantErr: "since:"},
		{name: "window invalid", window: "bad", wantErr: "window:"},
		{name: "reversed offsets", since: "2026-07-25T02:00:00+01:00", until: "2026-07-25T00:00:00Z", wantErr: "since (2026-07-25T02:00:00+01:00) is after until (2026-07-25T00:00:00Z)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			q, err := cfg.Resolve(Request{Author: "alice", Since: tc.since, Until: tc.until, Window: tc.window}, now)
			a, aerr := cfg.ResolveActivity(ActivityRequest{Repo: "a/b", Since: tc.since, Until: tc.until, Window: tc.window}, now)
			if tc.wantErr != "" {
				if err == nil || aerr == nil || err.Error() != aerr.Error() || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("inconsistent errors: query=%v activity=%v want=%s", err, aerr, tc.wantErr)
				}
				return
			}
			if err != nil || aerr != nil {
				t.Fatalf("query=%v activity=%v", err, aerr)
			}
			if q.Since != a.Since || q.Until != a.Until || q.Since.Location() != time.UTC || q.Until.Location() != time.UTC {
				t.Fatalf("window differs or not UTC: query=%+v activity=%+v", q, a)
			}
			if q.Since.Format(time.RFC3339) != tc.wantSince || q.Until.Format(time.RFC3339) != tc.wantUntil {
				t.Fatalf("got %s..%s, want %s..%s", q.Since, q.Until, tc.wantSince, tc.wantUntil)
			}
		})
	}
}
