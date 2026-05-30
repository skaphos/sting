// SPDX-License-Identifier: MIT
package cli

import (
	"bufio"
	"errors"
	"fmt"
	"strings"

	"github.com/skaphos/sting/internal/mcpinstall"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove sting MCP server entries from runtime configs",
	Long: "Removes the `sting` MCP entry from each detected (or selected) runtime config at the chosen scope.\n\n" +
		"Prompts once before deleting unless --yes is set. No-ops when nothing is present to remove.",
	RunE: runUninstall,
}

func init() {
	addRuntimeFlags(uninstallCmd)
	uninstallCmd.Flags().String("scope", "user", "config scope (user|project)")
	uninstallCmd.Flags().Bool("yes", false, "remove without interactive confirmation")
}

// uninstallTarget pairs a runtime with the resolved config path its entry lives at.
type uninstallTarget struct {
	runtime mcpinstall.Runtime
	path    string
}

func runUninstall(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()
	scopeStr, _ := f.GetString("scope")
	scope, err := parseInstallScope(scopeStr)
	if err != nil {
		return err
	}

	sel := runtimeSelection(cmd)
	explicit := len(sel.Explicit) > 0
	runtimes, err := sel.Resolve()
	if err != nil {
		return err
	}
	if len(runtimes) == 0 {
		return errors.New("no MCP-capable runtime detected; pass --claude, --codex, --opencode, or --grok explicitly")
	}

	targets, err := collectUninstallTargets(runtimes, scope, explicit)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}

	yes, _ := f.GetBool("yes")
	if !yes {
		ok, err := confirm(cmd, fmt.Sprintf("Remove sting MCP entry from %d config(s)? [y/N]: ", len(targets)))
		if err != nil {
			return err
		}
		if !ok {
			cmd.Println("uninstall cancelled")
			return nil
		}
	}

	for _, t := range targets {
		removed, err := t.runtime.RemoveEntry(t.path)
		if err != nil {
			return err
		}
		if removed {
			cmd.Printf("removed %s from %s\n", t.runtime.Name(), t.path)
		}
	}
	return nil
}

// collectUninstallTargets resolves config paths for each selected runtime,
// keeping only those that actually have our entry. Unsupported-scope errors
// surface only when the runtime was explicitly requested.
func collectUninstallTargets(runtimes []mcpinstall.Runtime, scope mcpinstall.Scope, explicit bool) ([]uninstallTarget, error) {
	var targets []uninstallTarget
	for _, r := range runtimes {
		path, err := r.ConfigPath(scope)
		if err != nil {
			if errors.Is(err, mcpinstall.ErrScopeUnsupported) {
				if explicit {
					return nil, fmt.Errorf("%s does not support --scope %s", r.Name(), scope)
				}
				continue
			}
			return nil, err
		}
		_, present, err := r.ReadEntry(path)
		if err != nil {
			return nil, err
		}
		if !present {
			continue
		}
		targets = append(targets, uninstallTarget{runtime: r, path: path})
	}
	return targets, nil
}

func confirm(cmd *cobra.Command, prompt string) (bool, error) {
	cmd.Print(prompt)
	reader := bufio.NewReader(cmd.InOrStdin())
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
