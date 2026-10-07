// Package cmd wires up the Cobra command tree. It depends only on
// internal/mcas, internal/config, internal/cache and internal/output -
// each command stays a thin adapter that fetches data and hands it to the
// output package for rendering.
package cmd

import (
	"github.com/spf13/cobra"
)

var (
	outputFormat string
	forceRefresh bool
	flagEmail    string
	flagPassword string
)

// version is stamped at release time via -ldflags "-X <module>/cmd.version=<tag>".
var version = "dev"

var rootCmd = &cobra.Command{
	Use:           "mcas",
	Version:       version,
	Short:         "Query My Child At School (MCAS) from the command line",
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the CLI. Errors have already been rendered to stdout in the
// requested output format by the failing command; the caller only needs
// the exit code.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&outputFormat, "output", "", `output format: "text" or "json" (default "text")`)
	rootCmd.PersistentFlags().BoolVar(&forceRefresh, "force-refresh", false, "bypass the local cache and fetch fresh data from MCAS")
	rootCmd.PersistentFlags().StringVar(&flagEmail, "email", "", "MCAS email (overrides stored credentials; shows up in shell history, prefer 'mcas login' or MCAS_EMAIL)")
	rootCmd.PersistentFlags().StringVar(&flagPassword, "password", "", "MCAS password (overrides stored credentials; shows up in shell history, prefer 'mcas login' or MCAS_PASSWORD)")
}
