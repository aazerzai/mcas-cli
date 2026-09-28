package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/dental-dash/my-child-at-school-cli/internal/mcas"
)

var attendanceDate string

var attendanceCmd = &cobra.Command{
	Use:   "attendance",
	Short: "Show registration marks for a day (default: today)",
	RunE:  runAttendance,
}

func init() {
	attendanceCmd.Flags().StringVar(&attendanceDate, "date", "", "date to fetch, YYYY-MM-DD (default: today)")
	rootCmd.AddCommand(attendanceCmd)
}

func runAttendance(cmd *cobra.Command, args []string) error {
	day, err := parseDateFlag(attendanceDate)
	if err != nil {
		return err
	}
	return runCommand("attendance", day.Format(dateFormat), func(client *mcas.Client) (*mcas.AttendanceDay, error) {
		return client.Attendance(day)
	}, renderAttendanceText)
}

func renderAttendanceText(w io.Writer, data any) error {
	day := data.(*mcas.AttendanceDay)
	present := "No data"
	if p := day.Present(); p != nil {
		if *p {
			present = "Present"
		} else {
			present = "Not present"
		}
	}
	_, err := fmt.Fprintf(w, "%s: %s\n%s\n", day.Day.Format(dateFormat), present, day.Summary())
	return err
}
