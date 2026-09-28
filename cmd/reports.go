package cmd

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/dental-dash/my-child-at-school-cli/internal/mcas"
)

var reportsCmd = &cobra.Command{
	Use:   "reports",
	Short: "Show school reports published to the parent",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCommand("reports", "all", func(client *mcas.Client) ([]map[string]any, error) {
			return client.Reports()
		}, func(w io.Writer, data any) error {
			return renderRawRows(w, data, "No reports published.")
		})
	},
}

func init() {
	rootCmd.AddCommand(reportsCmd)
}
