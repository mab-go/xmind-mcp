// Package main is the main package for the xmind-mcp application.
package main

import (
	"fmt"
	"strings"

	"github.com/mab-go/xmind-mcp/internal/logging"
	"github.com/mab-go/xmind-mcp/internal/server"
	"github.com/mab-go/xmind-mcp/internal/version"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	cmd = &cobra.Command{
		Use:     "xmind-mcp",
		Short:   "XMind MCP Server",
		Long:    "An MCP server for reading and writing local XMind mind map files.",
		Version: fmt.Sprintf("xmind-mcp %s (%s; %s)", version.Version, version.ShortCommit(), version.Date),
		RunE: func(_ *cobra.Command, _ []string) error {
			return server.RunStdioServer()
		},
	}
)

// init registers cobra/viper setup hooks, normalizes global flag names (underscores to
// hyphens), and configures the version output template.
func init() {
	cobra.OnInitialize(func() {
		viper.SetEnvPrefix("mcp_xmind")
		viper.AutomaticEnv()
	})
	cmd.SetGlobalNormalizationFunc(wordSepNormalizeFunc)
	cmd.SetVersionTemplate("{{.Version}}\n")
}

// wordSepNormalizeFunc normalizes flag names by replacing underscores with hyphens.
func wordSepNormalizeFunc(_ *pflag.FlagSet, name string) pflag.NormalizedName {
	name = strings.ReplaceAll(name, "_", "-")
	return pflag.NormalizedName(name)
}

// main runs the root cobra command; on failure it logs a fatal error and exits.
func main() {
	if err := cmd.Execute(); err != nil {
		logging.WithError(err).Fatal("Failed to execute root command")
	}
}
