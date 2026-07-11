// SPDX-License-Identifier: MIT
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	"github.com/skaphos/sting/internal/mcpinstall"
	"github.com/skaphos/sting/internal/mcpserver"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Register sting as an MCP server in your agent runtimes",
	Long: "Writes a `sting mcp` entry into the config of each detected (or selected) runtime.\n\n" +
		"Auto-detects Claude Code, Codex, OpenCode, and Grok by default; use --claude/--codex/--opencode/--grok to restrict targets. " +
		"--scope project writes the current directory's project config instead of the user config. " +
		"--command overrides the binary path written (defaults to the current executable). " +
		"sting's get_commits tool is read-only, so the Claude snippet also offers a paste-ready permissions allow-list.",
	RunE: runInstall,
}

func init() {
	addRuntimeFlags(installCmd)
	installCmd.Flags().String("scope", "user", "config scope (user|project)")
	installCmd.Flags().String("command", "", "binary path to write into config (default: os.Executable())")
	installCmd.Flags().String("manual", "", "print config snippet(s) to stdout instead of writing (all|claude|codex|opencode|grok)")
	installCmd.Flags().Lookup("manual").NoOptDefVal = "all"
	installCmd.AddCommand(installListCmd)
}

func addRuntimeFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("claude", false, "target Claude Code")
	cmd.Flags().Bool("codex", false, "target Codex")
	cmd.Flags().Bool("opencode", false, "target OpenCode")
	cmd.Flags().Bool("grok", false, "target Grok")
}

func runInstall(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()
	override, _ := f.GetString("command")

	if f.Changed("manual") {
		target, _ := f.GetString("manual")
		desired, err := desiredInstallEntry(override)
		if err != nil {
			return err
		}
		return printManualSnippets(cmd.OutOrStdout(), target, desired)
	}

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

	desired, err := desiredInstallEntry(override)
	if err != nil {
		return err
	}

	// Install into every selected runtime; a failure on one runtime is collected
	// and reported but never aborts the others.
	var errs []error
	for _, r := range runtimes {
		path, err := r.ConfigPath(scope)
		if err != nil {
			if errors.Is(err, mcpinstall.ErrScopeUnsupported) {
				if explicit {
					errs = append(errs, fmt.Errorf("%s does not support --scope %s", r.Name(), scope))
				}
				continue
			}
			errs = append(errs, fmt.Errorf("%s: %w", r.Name(), err))
			continue
		}
		existing, present, err := r.ReadEntry(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", r.Name(), err))
			continue
		}
		// Respect a user who deliberately disabled the entry: reinstalling must
		// not silently flip enabled back to true.
		d := desired
		if present && !existing.Enabled {
			d.Enabled = false
		}
		switch {
		case !present:
			if err := r.WriteEntry(path, d); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", r.Name(), err))
				continue
			}
			cmd.Printf("registered %s at %s\n", r.Name(), path)
		case reflect.DeepEqual(existing, d):
			cmd.Printf("unchanged %s at %s\n", r.Name(), path)
		default:
			if err := r.WriteEntry(path, d); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", r.Name(), err))
				continue
			}
			cmd.Printf("updated %s at %s\n", r.Name(), path)
		}
	}
	return errors.Join(errs...)
}

func runtimeSelection(cmd *cobra.Command) mcpinstall.Selection {
	f := cmd.Flags()
	claude, _ := f.GetBool("claude")
	codex, _ := f.GetBool("codex")
	opencode, _ := f.GetBool("opencode")
	grok, _ := f.GetBool("grok")
	return mcpinstall.SelectionFromFlags(claude, codex, opencode, grok)
}

func parseInstallScope(s string) (mcpinstall.Scope, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "user":
		return mcpinstall.ScopeUser, nil
	case "project":
		return mcpinstall.ScopeProject, nil
	default:
		return 0, fmt.Errorf("invalid --scope %q (want user or project)", s)
	}
}

// manualTargets are the runtime names install --manual emits, in order.
var manualTargets = []string{"claude", "codex", "opencode", "grok"}

func printManualSnippets(w io.Writer, target string, desired mcpinstall.Entry) error {
	key := strings.ToLower(strings.TrimSpace(target))
	if key == "" {
		key = "all"
	}
	var names []string
	switch key {
	case "all":
		names = manualTargets
	case "claude", "codex", "opencode", "grok":
		names = []string{key}
	default:
		return fmt.Errorf("invalid --manual %q (want all|claude|codex|opencode|grok)", target)
	}
	for i, name := range names {
		snippet, err := mcpinstall.Snippet(name, desired)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "# %s\n%s", name, snippet); err != nil {
			return err
		}
		if name == "claude" {
			if err := printClaudePermissionsBlock(w); err != nil {
				return err
			}
		}
		if i < len(names)-1 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
	}
	return nil
}

// printClaudePermissionsBlock emits a paste-ready permissions.allow snippet for
// ~/.claude/settings.json that auto-approves sting's read-only tools. The list
// is derived from the live server annotations so it cannot drift.
func printClaudePermissionsBlock(w io.Writer) error {
	permissions, err := mcpinstall.ClaudePermissionsSnippet(mcpserver.ReadOnlyTools())
	if err != nil {
		return err
	}
	if _, err := fmt.Fprint(w,
		"\n# claude — recommended ~/.claude/settings.json permissions\n"+
			"# sting's tools are read-only, so auto-approving them is safe.\n",
	); err != nil {
		return err
	}
	_, err = fmt.Fprint(w, permissions)
	return err
}

func desiredInstallEntry(override string) (mcpinstall.Entry, error) {
	bin := strings.TrimSpace(override)
	if bin == "" {
		exe, err := os.Executable()
		if err != nil {
			return mcpinstall.Entry{}, fmt.Errorf("resolve binary path: %w", err)
		}
		bin = exe
	}
	return mcpinstall.Entry{Command: bin, Args: []string{"mcp"}, Enabled: true}, nil
}
