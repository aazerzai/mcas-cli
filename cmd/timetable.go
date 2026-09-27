package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/dental-dash/my-child-at-school-cli/internal/mcas"
)

var timetableCmd = &cobra.Command{
	Use:   "timetable",
	Short: "Show this week's timetable",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCommand("timetable", "week", func(client *mcas.Client) ([]mcas.Lesson, error) {
			return client.Timetable()
		}, renderTimetableText)
	},
}

func init() {
	rootCmd.AddCommand(timetableCmd)
}

func renderTimetableText(w io.Writer, data any) error {
	lessons := data.([]mcas.Lesson)
	if len(lessons) == 0 {
		_, err := fmt.Fprintln(w, "No timetable data available.")
		return err
	}
	for _, lesson := range lessons {
		date := ""
		if lesson.Date != nil {
			date = " (" + lesson.Date.Format(dateFormat) + ")"
		}
		if _, err := fmt.Fprintf(w, "%s%s %s: %s - %s, %s\n", lesson.Day, date, lesson.Period, lesson.Subject, lesson.Class, lesson.Teacher); err != nil {
			return err
		}
	}
	return nil
}
