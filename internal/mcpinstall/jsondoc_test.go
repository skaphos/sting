// SPDX-License-Identifier: MIT
package mcpinstall

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReadJSONDocRequiresEOF(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{name: "object", content: `{"mcpServers":{}}`},
		{name: "trailing whitespace", content: "{\"mcpServers\":{}} \n\t"},
		{name: "second object", content: "{\"mcpServers\":{}}\n{\"other\":true}", wantErr: true},
		{name: "trailing primitive", content: `{"mcpServers":{}} true`, wantErr: true},
		{name: "trailing garbage", content: `{"mcpServers":{}} garbage`, wantErr: true},
		{name: "top-level primitive", content: `true`, wantErr: true},
		{name: "top-level null", content: `null`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := readJSONDoc(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("readJSONDoc error = %v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestJSONAdaptersPreserveMalformedTrailingData(t *testing.T) {
	original := []byte("{\"mcpServers\":{}}\n{\"other\":\"valuable configuration\"}\n")
	for _, name := range []string{"claude", "opencode"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			r, _ := ByName(name)
			if err := r.WriteEntry(path, Entry{Command: "sting"}); err == nil {
				t.Fatal("WriteEntry accepted trailing JSON data")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, original) {
				t.Fatalf("WriteEntry changed malformed input:\ngot:  %q\nwant: %q", got, original)
			}
		})
	}
}
