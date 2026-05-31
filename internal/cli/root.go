// SPDX-License-Identifier: MIT
// Package cli is the cobra command tree for the sting CLI: querying commits,
// running the MCP server, and installing/uninstalling the server into agent
// runtimes. Configuration is resolved with viper (defaults < config file < env
// < flags) so dedicated provider PATs can live in sting's own config rather
// than relying on ambient provider tokens.
package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/skaphos/sting/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// envPrefix namespaces environment overrides (e.g. STING_TOKEN, STING_WINDOW)
// so sting's PATs stay distinct from ambient provider tokens.
const envPrefix = "STING"

var (
	v          = viper.New()
	configFile string
)

var rootCmd = &cobra.Command{
	Use:   "sting",
	Short: "Query a GitHub or GitLab user's commits over a time window",
	Long: "sting reports a GitHub or GitLab user's commits over a time window for an LLM agent or a terminal.\n\n" +
		"Run with query flags to print a report, `sting mcp` to serve the read-only get_commits tool over stdio, " +
		"or `sting install` to register that server with your agent runtimes.",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runQuery,
}

func init() {
	cobra.OnInitialize(initConfig)

	pf := rootCmd.PersistentFlags()
	pf.StringVar(&configFile, "config", "", "config file (default: search XDG/HOME for sting/config.yaml)")
	pf.String("token", "", "GitHub personal access token (overrides config/env)")
	pf.String("base-url", "", "GitHub Enterprise API base URL")
	pf.String("gitlab-token", "", "GitLab personal access token (overrides config/env)")
	pf.String("gitlab-base-url", "", "GitLab API v4 base URL")
	pf.Int("per-page", 100, "API page size (1-100)")
	pf.Int("max-commits", 0, "cap on returned commits (0 = unlimited)")

	// Bind the config-bearing persistent flags to their viper keys so flags win
	// over env and file when set.
	must(v.BindPFlag("token", pf.Lookup("token")))
	must(v.BindPFlag("base_url", pf.Lookup("base-url")))
	must(v.BindPFlag("gitlab_token", pf.Lookup("gitlab-token")))
	must(v.BindPFlag("gitlab_base_url", pf.Lookup("gitlab-base-url")))
	must(v.BindPFlag("per_page", pf.Lookup("per-page")))
	must(v.BindPFlag("max_commits", pf.Lookup("max-commits")))

	registerQueryFlags(rootCmd)

	rootCmd.AddCommand(mcpCmd, installCmd, uninstallCmd, versionCmd, authCmd, initCmd)
}

// initConfig seeds defaults, wires environment overrides, and reads the config
// file. It runs after flag parsing via cobra.OnInitialize.
func initConfig() {
	for key, val := range config.Defaults() {
		v.SetDefault(key, val)
	}

	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()

	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		for _, dir := range configSearchDirs() {
			v.AddConfigPath(dir)
		}
	}

	if err := v.ReadInConfig(); err != nil && !configMissing(err) {
		// A missing config file is fine (sting works from defaults/env/flags); a
		// real parse error is worth surfacing without aborting the command.
		fmt.Fprintln(os.Stderr, "sting: warning: "+err.Error())
	}
}

// configMissing reports whether err is "no config file found", whether from
// auto-discovery (ConfigFileNotFoundError) or an explicit --config path that
// does not exist (a PathError).
func configMissing(err error) bool {
	var notFound viper.ConfigFileNotFoundError
	return errors.As(err, &notFound) || os.IsNotExist(err)
}

// configSearchDirs returns the directories searched for config.yaml, most
// specific first.
func configSearchDirs() []string {
	var dirs []string
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		dirs = append(dirs, filepath.Join(xdg, "sting"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(home, ".config", "sting"),
			filepath.Join(home, ".sting"),
		)
	}
	dirs = append(dirs, ".")
	return dirs
}

// loadConfig unmarshals the resolved viper state into a validated Config.
func loadConfig() (config.Config, error) {
	var cfg config.Config
	if err := v.Unmarshal(&cfg); err != nil {
		return config.Config{}, fmt.Errorf("decode config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

// Execute runs the root command and exits with a shell-friendly status.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "sting: "+err.Error())
		os.Exit(1)
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
