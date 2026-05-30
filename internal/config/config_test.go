package config

import (
	"testing"
	"time"
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
