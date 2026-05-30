// SPDX-License-Identifier: MIT
package cli

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skaphos/sting/internal/mcpserver"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run the MCP server over stdio",
	Long: "Serves the read-only get_commits tool over stdio for an LLM agent.\n\n" +
		"This is what `sting install` registers in each runtime. Stdout is owned by " +
		"the MCP protocol, so do not mix it with other output.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		server, err := mcpserver.New(cfg)
		if err != nil {
			return err
		}
		return server.Run(cmd.Context(), &mcp.StdioTransport{})
	},
}
