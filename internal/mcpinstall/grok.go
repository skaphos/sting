package mcpinstall

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type grokAdapter struct{}

func init() { register(&grokAdapter{}) }

func (a *grokAdapter) Name() string { return "grok" }

// grokDir resolves Grok's user-scope config directory: GROK_CONFIG_DIR wins,
// otherwise ~/.grok.
func grokDir() (string, error) {
	if v := os.Getenv("GROK_CONFIG_DIR"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".grok"), nil
}

func (a *grokAdapter) Detect() (bool, error) {
	if os.Getenv("GROK_CONFIG_DIR") != "" {
		return true, nil
	}
	dir, err := grokDir()
	if err != nil {
		return false, err
	}
	for _, p := range []string{filepath.Join(dir, "config.toml"), dir} {
		if _, err := os.Stat(p); err == nil {
			return true, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}

func (a *grokAdapter) ConfigPath(scope Scope) (string, error) {
	switch scope {
	case ScopeUser:
		dir, err := grokDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "config.toml"), nil
	case ScopeProject:
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(cwd, ".grok", "config.toml"), nil
	default:
		return "", fmt.Errorf("unknown scope: %v", scope)
	}
}

// grokServer is the typed TOML shape of a Grok mcp_servers entry. Grok requires
// an explicit enabled boolean.
type grokServer struct {
	Command string            `toml:"command"`
	Args    []string          `toml:"args,omitempty"`
	Enabled bool              `toml:"enabled"`
	Env     map[string]string `toml:"env,omitempty"`
}

func (a *grokAdapter) ReadEntry(path string) (Entry, bool, error) {
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
	var srv grokServer
	if err := decodeTOMLInto(raw, &srv, path, "mcp_servers."+serverKey); err != nil {
		return Entry{}, false, err
	}
	return Entry{Command: srv.Command, Args: srv.Args, Enabled: srv.Enabled}, true, nil
}

func (a *grokAdapter) WriteEntry(path string, e Entry) error {
	doc, err := readTOMLDoc(path)
	if err != nil {
		return err
	}
	servers, err := tomlTableAt(doc, "mcp_servers", path)
	if err != nil {
		return err
	}
	if servers == nil {
		servers = map[string]any{}
	}
	servers[serverKey] = grokServer{Command: e.Command, Args: e.Args, Enabled: e.Enabled}
	doc["mcp_servers"] = servers
	return writeTOMLDoc(path, doc, 0o644)
}

func (a *grokAdapter) RemoveEntry(path string) (bool, error) {
	doc, err := readTOMLDoc(path)
	if err != nil {
		return false, err
	}
	servers, err := tomlTableAt(doc, "mcp_servers", path)
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
	doc["mcp_servers"] = servers
	return true, writeTOMLDoc(path, doc, 0o644)
}
