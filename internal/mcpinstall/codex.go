// SPDX-License-Identifier: MIT
package mcpinstall

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type codexAdapter struct{}

func init() { register(&codexAdapter{}) }

func (a *codexAdapter) Name() string { return "codex" }

func (a *codexAdapter) Detect() (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(filepath.Join(home, ".codex")); err == nil {
		return true, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	return false, nil
}

func (a *codexAdapter) ConfigPath(scope Scope) (string, error) {
	switch scope {
	case ScopeUser:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".codex", "config.toml"), nil
	case ScopeProject:
		return "", ErrScopeUnsupported
	default:
		return "", fmt.Errorf("unknown scope: %v", scope)
	}
}

// codexServer is the typed TOML shape of an mcp_servers entry.
type codexServer struct {
	Command string   `toml:"command"`
	Args    []string `toml:"args,omitempty"`
}

func (a *codexAdapter) ReadEntry(path string) (Entry, bool, error) {
	doc, err := readTOMLDoc(path)
	if err != nil {
		return Entry{}, false, err
	}
	servers, err := tomlTableAt(doc, "mcp_servers", path)
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
	var srv codexServer
	if err := decodeTOMLInto(raw, &srv, path, "mcp_servers."+serverKey); err != nil {
		return Entry{}, false, err
	}
	return Entry{Command: srv.Command, Args: srv.Args, Enabled: true}, true, nil
}

func (a *codexAdapter) WriteEntry(path string, e Entry) error {
	set := map[string]any{"command": e.Command}
	if len(e.Args) > 0 {
		set["args"] = e.Args
	} else {
		set["args"] = nil
	}
	return upsertTOMLServer(path, set, 0o644)
}

func (a *codexAdapter) RemoveEntry(path string) (bool, error) {
	return deleteTOMLServer(path, 0o644)
}

// --- shared TOML helpers (Codex + Grok) ---

func readTOMLDoc(path string) (map[string]any, error) {
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
	if err := toml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse %q: %w", path, err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, nil
}

func tomlTableAt(doc map[string]any, key, path string) (map[string]any, error) {
	raw, ok := doc[key]
	if !ok {
		return nil, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parse %q: %s is not a TOML table (got %T)", path, key, raw)
	}
	return m, nil
}

func decodeTOMLInto(raw any, dst any, path, field string) error {
	m, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("parse %q: %s is not a TOML table (got %T)", path, field, raw)
	}
	b, err := toml.Marshal(m)
	if err != nil {
		return fmt.Errorf("parse %q: %s: %w", path, field, err)
	}
	if err := toml.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("parse %q: %s: %w", path, field, err)
	}
	return nil
}
