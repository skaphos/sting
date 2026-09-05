// SPDX-License-Identifier: MIT
package config

import (
	"fmt"
	"time"
)

// resolveWindow applies shared precedence and normalizes successful windows to
// UTC. Validate before normalization to preserve input offsets in error messages.
func resolveWindow(sinceInput, untilInput, window, defaultWindow string, now time.Time) (time.Time, time.Time, error) {
	until := now
	if untilInput != "" {
		t, err := ParseTime(untilInput)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("until: %w", err)
		}
		until = t
	}
	var since time.Time
	if sinceInput != "" {
		t, err := ParseTime(sinceInput)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("since: %w", err)
		}
		since = t
	} else {
		if window == "" {
			window = defaultWindow
		}
		d, err := ParseWindow(window)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("window: %w", err)
		}
		since = until.Add(-d)
	}
	if since.After(until) {
		return time.Time{}, time.Time{}, fmt.Errorf("since (%s) is after until (%s)",
			since.Format(time.RFC3339), until.Format(time.RFC3339))
	}
	return since.UTC(), until.UTC(), nil
}
