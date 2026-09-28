package cmd

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/dental-dash/my-child-at-school-cli/internal/mcas"
)

var detentionsCmd = &cobra.Command{
	Use:   "detentions",
	Short: "Show recorded detentions",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCommand("detentions", "all", func(client *mcas.Client) ([]map[string]any, error) {
			return client.Detentions()
		}, func(w io.Writer, data any) error {
			return renderRawRows(w, data, "No detentions recorded.")
		})
	},
}

func init() {
	rootCmd.AddCommand(detentionsCmd)
}
