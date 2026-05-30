// SPDX-License-Identifier: MIT
package render

import (
	"strings"
	"testing"
)

func TestRenderUnknownFormat(t *testing.T) {
	if _, err := Render(sampleResult(), Format("xml")); err == nil {
		t.Error("Render with unknown format: want error, got nil")
	}
}

func TestRenderJSONSuccess(t *testing.T) {
	out, err := Render(sampleResult(), FormatJSON)
	if err != nil {
		t.Fatalf("Render json: %v", err)
	}
	if !strings.Contains(out, `"author": "mendedlink"`) {
		t.Errorf("JSON output missing author field:\n%s", out)
	}
	if !strings.HasPrefix(out, "{") {
		t.Errorf("JSON output should be an object, got: %q", out)
	}
}
