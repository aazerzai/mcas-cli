package cmd

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/dental-dash/my-child-at-school-cli/internal/mcas"
)

var clubsCmd = &cobra.Command{
	Use:   "clubs",
	Short: "Show clubs and trips the pupil is enrolled on",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCommand("clubs", "all", func(client *mcas.Client) ([]map[string]any, error) {
			return client.ClubsAndTrips()
		}, func(w io.Writer, data any) error {
			return renderRawRows(w, data, "No clubs or trips recorded.")
		})
	},
}

func init() {
	rootCmd.AddCommand(clubsCmd)
}
