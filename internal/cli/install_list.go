// SPDX-License-Identifier: MIT
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/skaphos/sting/internal/mcpinstall"
	"github.com/spf13/cobra"
)

var installListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show whether sting is registered with each runtime",
	Long: "Reports each runtime's registration state at the chosen scope.\n\n" +
		"'registered' means the config points at the current executable; 'registered (stale)' means it points at a " +
		"different binary (e.g. after an upgrade moved the path); 'not registered' means no entry; 'unsupported' means " +
		"the runtime has no config for that scope (Codex under --scope project).",
	RunE: runInstallList,
}

func init() {
	installListCmd.Flags().String("scope", "user", "config scope (user|project)")
	installListCmd.Flags().Bool("json", false, "emit JSON instead of a table")
}

// listRow is the per-runtime record; JSON tags double as the --json schema.
type listRow struct {
	Name    string `json:"name"`
	Scope   string `json:"scope"`
	Path    string `json:"path"`
	State   string `json:"state"`
	Command string `json:"command,omitempty"`
}

func runInstallList(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()
	scopeStr, _ := f.GetString("scope")
	asJSON, _ := f.GetBool("json")

	scope, err := parseInstallScope(scopeStr)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve binary path: %w", err)
	}

	rows := make([]listRow, 0, len(mcpinstall.All()))
	for _, r := range mcpinstall.All() {
		row := listRow{Name: r.Name(), Scope: scope.String()}
		path, err := r.ConfigPath(scope)
		if err != nil {
			if errors.Is(err, mcpinstall.ErrScopeUnsupported) {
				row.State = "unsupported"
				rows = append(rows, row)
				continue
			}
			return err
		}
		row.Path = path
		entry, present, err := r.ReadEntry(path)
		if err != nil {
			return err
		}
		switch {
		case !present:
			row.State = "not registered"
		case entry.Command != exe:
			row.State = "registered (stale)"
			row.Command = entry.Command
		default:
			row.State = "registered"
			row.Command = entry.Command
		}
		rows = append(rows, row)
	}

	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"scope": scope.String(), "runtimes": rows})
	}
	return writeInstallListTable(cmd.OutOrStdout(), rows)
}

func writeInstallListTable(w io.Writer, rows []listRow) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "NAME\tSCOPE\tPATH\tSTATE\tCOMMAND"); err != nil {
		return err
	}
	for _, r := range rows {
		cmdStr := dash(r.Command)
		path := dash(r.Path)
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.Name, r.Scope, path, r.State, cmdStr); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
