// SPDX-License-Identifier: MIT
package mcpinstall

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type claudeAdapter struct{}

func init() { register(&claudeAdapter{}) }

func (a *claudeAdapter) Name() string { return "claude" }

func (a *claudeAdapter) Detect() (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	for _, p := range []string{filepath.Join(home, ".claude.json"), filepath.Join(home, ".claude")} {
		if _, err := os.Stat(p); err == nil {
			return true, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}

func (a *claudeAdapter) ConfigPath(scope Scope) (string, error) {
	switch scope {
	case ScopeUser:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".claude.json"), nil
	case ScopeProject:
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(cwd, ".mcp.json"), nil
	default:
		return "", fmt.Errorf("unknown scope: %v", scope)
	}
}

// claudeServer is the typed JSON shape of an mcpServers entry.
type claudeServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

func (a *claudeAdapter) ReadEntry(path string) (Entry, bool, error) {
	doc, err := readJSONDoc(path)
	if err != nil {
		return Entry{}, false, err
	}
	servers, err := jsonObjectAt(doc, "mcpServers", path)
	if err != nil {
		return Entry{}, false, err
	}
	if servers == nil {
		return Entry{}, false, nil
	}
	raw, ok := servers[serverKey]
	if !ok {
		return Entry{}, false, nil
	}
	var srv claudeServer
	if err := decodeJSONInto(raw, &srv, path, "mcpServers."+serverKey); err != nil {
		return Entry{}, false, err
	}
	return Entry{Command: srv.Command, Args: srv.Args, Enabled: true}, true, nil
}

func (a *claudeAdapter) WriteEntry(path string, e Entry) error {
	doc, err := readJSONDoc(path)
	if err != nil {
		return err
	}
	servers, err := jsonObjectAt(doc, "mcpServers", path)
	if err != nil {
		return err
	}
	if servers == nil {
		servers = map[string]any{}
	}
	servers[serverKey] = claudeServer{Command: e.Command, Args: e.Args}
	doc["mcpServers"] = servers
	return writeJSONDoc(path, doc, 0o644)
}

func (a *claudeAdapter) RemoveEntry(path string) (bool, error) {
	doc, err := readJSONDoc(path)
	if err != nil {
		return false, err
	}
	servers, err := jsonObjectAt(doc, "mcpServers", path)
	if err != nil {
		return false, err
	}
	if servers == nil {
		return false, nil
	}
	if _, ok := servers[serverKey]; !ok {
		return false, nil
	}
	delete(servers, serverKey)
	doc["mcpServers"] = servers
	return true, writeJSONDoc(path, doc, 0o644)
}

// --- shared JSON helpers (Claude + OpenCode) ---

// readJSONDoc parses path as a top-level JSON object. Missing or empty files
// yield an empty doc; malformed files yield a wrapped parse error.
func readJSONDoc(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse %q: %w", path, err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, nil
}

// writeJSONDoc marshals doc with 2-space indent, ensures the parent directory
// exists, and writes atomically.
func writeJSONDoc(path string, doc map[string]any, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return WriteAtomic(path, raw, mode)
}

// jsonObjectAt returns doc[key] as a JSON object, nil if absent, or an error if
// present but not an object.
func jsonObjectAt(doc map[string]any, key, path string) (map[string]any, error) {
	raw, ok := doc[key]
	if !ok {
		return nil, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parse %q: %s is not a JSON object (got %T)", path, key, raw)
	}
	return m, nil
}

// decodeJSONInto round-trips a parsed JSON value into a typed struct.
func decodeJSONInto(raw any, dst any, path, field string) error {
	m, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("parse %q: %s is not a JSON object (got %T)", path, field, raw)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("parse %q: %s: %w", path, field, err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("parse %q: %s: %w", path, field, err)
	}
	return nil
}
