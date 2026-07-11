// SPDX-License-Identifier: MIT
// Package mcpinstall registers and removes sting's MCP server entry in the
// config files of agent runtimes (Claude Code, Codex, OpenCode, Grok). Each
// runtime has an adapter translating the runtime-agnostic Entry into that
// runtime's native config shape. Writes are atomic (temp file + rename with
// fsync) and preserve unrelated content: TOML files are edited surgically as
// text so comments, ordering, and quoting survive; JSON files are merged
// structurally so every other key (and any extra keys on sting's own entry) is
// kept.
package mcpinstall

import (
	"errors"
	"fmt"
	"sort"
)

// serverKey is the MCP server name sting registers under in every runtime's
// config file.
const serverKey = "sting"

// Scope selects user-scoped or project-scoped config files.
type Scope int

const (
	ScopeUser Scope = iota
	ScopeProject
)

func (s Scope) String() string {
	switch s {
	case ScopeUser:
		return "user"
	case ScopeProject:
		return "project"
	default:
		return "unknown"
	}
}

// Entry is the runtime-agnostic MCP server entry written into each agent's
// config. Adapters translate it to their native shape (JSON object vs TOML
// table, command+args vs argv array). Enabled is honored by runtimes that have
// the concept (OpenCode, Grok); others always write it as true.
type Entry struct {
	Command string
	Args    []string
	Enabled bool
}

// ErrScopeUnsupported is returned by adapters that do not support a given Scope
// (e.g. Codex has no project scope).
var ErrScopeUnsupported = errors.New("scope not supported by this runtime")

// Runtime is the adapter contract; implementations live in the per-runtime
// files in this package.
type Runtime interface {
	Name() string
	Detect() (bool, error)
	ConfigPath(scope Scope) (string, error)
	ReadEntry(path string) (entry Entry, present bool, err error)
	WriteEntry(path string, entry Entry) error
	RemoveEntry(path string) (removed bool, err error)
}

var registered []Runtime

func register(r Runtime) { registered = append(registered, r) }

// All returns the registered adapters sorted by name for deterministic output.
func All() []Runtime {
	out := make([]Runtime, len(registered))
	copy(out, registered)
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// ByName looks up a single adapter by canonical name.
func ByName(name string) (Runtime, bool) {
	for _, r := range registered {
		if r.Name() == name {
			return r, true
		}
	}
	return nil, false
}

// Selection expresses which runtimes a CLI invocation targets. An empty
// Explicit list means auto-detect; otherwise only the named runtimes apply.
type Selection struct {
	Explicit []string
}

// SelectionFromFlags builds a Selection from per-runtime bool flags, in a
// deterministic order.
func SelectionFromFlags(claude, codex, opencode, grok bool) Selection {
	s := Selection{}
	if claude {
		s.Explicit = append(s.Explicit, "claude")
	}
	if codex {
		s.Explicit = append(s.Explicit, "codex")
	}
	if opencode {
		s.Explicit = append(s.Explicit, "opencode")
	}
	if grok {
		s.Explicit = append(s.Explicit, "grok")
	}
	return s
}

// Resolve returns the adapters this Selection targets. An empty Selection
// filters registered adapters by Detect(); an explicit list is honored even for
// runtimes Detect() would skip.
func (s Selection) Resolve() ([]Runtime, error) {
	if len(s.Explicit) == 0 {
		var present []Runtime
		for _, r := range All() {
			ok, err := r.Detect()
			if err != nil {
				return nil, err
			}
			if ok {
				present = append(present, r)
			}
		}
		return present, nil
	}
	out := make([]Runtime, 0, len(s.Explicit))
	for _, name := range s.Explicit {
		r, ok := ByName(name)
		if !ok {
			return nil, fmt.Errorf("unknown runtime: %q", name)
		}
		out = append(out, r)
	}
	return out, nil
}
