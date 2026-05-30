// Package main is the main package for the xmind-mcp application.
package main

import (
	"fmt"
	"strings"

	"github.com/mab-go/logging"
	"github.com/mab-go/xmind-mcp/internal/server"
	"github.com/mab-go/xmind-mcp/internal/version"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const eventCmdFail logging.Event = "cmd.fail" // Failed to execute root command

var (
	cmd = &cobra.Command{
		Use:     "xmind-mcp",
		Short:   "XMind MCP Server",
		Long:    "An MCP server for reading and writing local XMind mind map files.",
		Version: fmt.Sprintf("xmind-mcp %s (%s; %s)", version.Version, version.ShortCommit(), version.Date),
		RunE: func(_ *cobra.Command, _ []string) error {
			lvl, err := parseLogLevel(viper.GetString("log-level"))
			if err != nil {
				return err
			}
			logging.SetDefaultConfig(logging.Config{Level: lvl})

			return server.RunStdioServer()
		},
	}
)

// init registers cobra/viper setup hooks, normalizes global flag names (underscores to
// hyphens), and configures the version output template.
func init() {
	cobra.OnInitialize(func() {
		viper.SetEnvPrefix("xmind_mcp")
		// Map hyphenated keys to underscored env vars, e.g. the "log-level"
		// key resolves to XMIND_MCP_LOG_LEVEL.
		viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
		viper.AutomaticEnv()
		_ = viper.BindPFlag("log-level", cmd.PersistentFlags().Lookup("log-level"))
	})
	cmd.PersistentFlags().String("log-level", "info", "log level: debug, info, warn, error")
	cmd.SetGlobalNormalizationFunc(wordSepNormalizeFunc)
	cmd.SetVersionTemplate("{{.Version}}\n")
}

// parseLogLevel converts a case-insensitive level name to a logging.Level. It
// returns an error for unrecognized names so an invalid --log-level / env value
// fails fast at startup.
func parseLogLevel(s string) (logging.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return logging.DebugLevel, nil
	case "info":
		return logging.InfoLevel, nil
	case "warn":
		return logging.WarnLevel, nil
	case "error":
		return logging.ErrorLevel, nil
	default:
		return 0, fmt.Errorf("invalid log level: %q (want debug, info, warn, or error)", s)
	}
}

// wordSepNormalizeFunc normalizes flag names by replacing underscores with hyphens.
func wordSepNormalizeFunc(_ *pflag.FlagSet, name string) pflag.NormalizedName {
	name = strings.ReplaceAll(name, "_", "-")
	return pflag.NormalizedName(name)
}

// main runs the root cobra command; on failure it logs a fatal error and exits.
func main() {
	if err := cmd.Execute(); err != nil {
		logging.WithError(err).Fatal(eventCmdFail)
	}
}
