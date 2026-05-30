// SPDX-License-Identifier: MIT
package mcpinstall

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type opencodeAdapter struct{}

func init() { register(&opencodeAdapter{}) }

func (a *opencodeAdapter) Name() string { return "opencode" }

// opencodeDir resolves OpenCode's user-scope config directory:
// OPENCODE_CONFIG_DIR overrides everything; otherwise XDG_CONFIG_HOME/opencode;
// otherwise ~/.config/opencode.
func opencodeDir() (string, error) {
	if v := os.Getenv("OPENCODE_CONFIG_DIR"); v != "" {
		return v, nil
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "opencode"), nil
}

func (a *opencodeAdapter) Detect() (bool, error) {
	if os.Getenv("OPENCODE_CONFIG_DIR") != "" {
		return true, nil
	}
	dir, err := opencodeDir()
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(dir); err == nil {
		return true, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	return false, nil
}

func (a *opencodeAdapter) ConfigPath(scope Scope) (string, error) {
	switch scope {
	case ScopeUser:
		dir, err := opencodeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "opencode.json"), nil
	case ScopeProject:
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(cwd, "opencode.json"), nil
	default:
		return "", fmt.Errorf("unknown scope: %v", scope)
	}
}

// opencodeServer is the typed JSON shape of an mcp entry. OpenCode's command is
// a single argv array, not a command string plus separate args.
type opencodeServer struct {
	Type    string   `json:"type"`
	Command []string `json:"command"`
	Enabled bool     `json:"enabled"`
}

// checkJsonc refuses to act on .jsonc paths or .json paths whose .jsonc sibling
// exists, so we never desync from the user's real config.
func checkJsonc(path string) error {
	if strings.HasSuffix(path, ".jsonc") {
		return fmt.Errorf("refusing to operate on %q: .jsonc not supported; rename to .json or edit manually", path)
	}
	if !strings.HasSuffix(path, ".json") {
		return nil
	}
	sibling := strings.TrimSuffix(path, ".json") + ".jsonc"
	if _, err := os.Stat(sibling); err == nil {
		return fmt.Errorf("refusing to operate on %q: a .jsonc sibling exists at %q; edit manually", path, sibling)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (a *opencodeAdapter) ReadEntry(path string) (Entry, bool, error) {
	if err := checkJsonc(path); err != nil {
		return Entry{}, false, err
	}
	doc, err := readJSONDoc(path)
	if err != nil {
		return Entry{}, false, err
	}
	servers, err := jsonObjectAt(doc, "mcp", path)
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
	var srv opencodeServer
	if err := decodeJSONInto(raw, &srv, path, "mcp."+serverKey); err != nil {
		return Entry{}, false, err
	}
	if len(srv.Command) == 0 {
		return Entry{}, false, fmt.Errorf("parse %q: mcp.%s.command is empty", path, serverKey)
	}
	return Entry{Command: srv.Command[0], Args: srv.Command[1:], Enabled: srv.Enabled}, true, nil
}

func (a *opencodeAdapter) WriteEntry(path string, e Entry) error {
	if err := checkJsonc(path); err != nil {
		return err
	}
	doc, err := readJSONDoc(path)
	if err != nil {
		return err
	}
	servers, err := jsonObjectAt(doc, "mcp", path)
	if err != nil {
		return err
	}
	if servers == nil {
		servers = map[string]any{}
	}
	argv := append([]string{e.Command}, e.Args...)
	servers[serverKey] = opencodeServer{Type: "local", Command: argv, Enabled: e.Enabled}
	doc["mcp"] = servers
	return writeJSONDoc(path, doc, 0o644)
}

func (a *opencodeAdapter) RemoveEntry(path string) (bool, error) {
	if err := checkJsonc(path); err != nil {
		return false, err
	}
	doc, err := readJSONDoc(path)
	if err != nil {
		return false, err
	}
	servers, err := jsonObjectAt(doc, "mcp", path)
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
	doc["mcp"] = servers
	return true, writeJSONDoc(path, doc, 0o644)
}
