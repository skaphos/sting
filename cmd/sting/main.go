// Command sting queries a GitHub user's commits over a time window, as a local
// CLI or as an MCP server for an LLM agent. The command tree lives in
// internal/cli so this entrypoint stays thin.
package main

import "github.com/skaphos/sting/internal/cli"

func main() {
	cli.Execute()
}
