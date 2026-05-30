// SPDX-License-Identifier: MIT
package patch

import "testing"

func TestConsumePatchBudget(t *testing.T) {
	tests := []struct {
		name      string
		patch     string
		budget    int
		wantPatch string
		wantTrunc bool
		wantRem   int
	}{
		{
			name:      "empty patch",
			patch:     "",
			budget:    100,
			wantPatch: "",
			wantTrunc: false,
			wantRem:   100,
		},
		{
			name:      "within budget",
			patch:     "@@ -1 +1 @@\n-old\n+new\n",
			budget:    100,
			wantPatch: "@@ -1 +1 @@\n-old\n+new\n",
			wantTrunc: false,
			wantRem:   100 - len("@@ -1 +1 @@\n-old\n+new\n"),
		},
		{
			name:      "exact budget",
			patch:     "12345",
			budget:    5,
			wantPatch: "12345",
			wantTrunc: false,
			wantRem:   0,
		},
		{
			name:      "exceeds budget",
			patch:     "1234567890",
			budget:    5,
			wantPatch: "12345",
			wantTrunc: true,
			wantRem:   0,
		},
		{
			name:      "already exhausted budget",
			patch:     "anything",
			budget:    0,
			wantPatch: "",
			wantTrunc: true,
			wantRem:   0,
		},
		{
			name:      "negative budget treated as exhausted",
			patch:     "foo",
			budget:    -10,
			wantPatch: "",
			wantTrunc: true,
			wantRem:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPatch, gotTrunc, gotRem := ConsumePatchBudget(tt.patch, tt.budget)
			if gotPatch != tt.wantPatch {
				t.Errorf("patch = %q, want %q", gotPatch, tt.wantPatch)
			}
			if gotTrunc != tt.wantTrunc {
				t.Errorf("truncated = %v, want %v", gotTrunc, tt.wantTrunc)
			}
			if gotRem != tt.wantRem {
				t.Errorf("remaining = %d, want %d", gotRem, tt.wantRem)
			}
		})
	}
}
