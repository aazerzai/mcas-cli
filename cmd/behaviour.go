package cmd

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dental-dash/my-child-at-school-cli/internal/mcas"
)

var behaviourDate string

var behaviourCmd = &cobra.Command{
	Use:   "behaviour",
	Short: "Show behaviour points and events for the academic year, or a single day with --date",
	RunE:  runBehaviour,
}

func init() {
	behaviourCmd.Flags().StringVar(&behaviourDate, "date", "", "show behaviour events for a single day, YYYY-MM-DD, instead of the year summary")
	rootCmd.AddCommand(behaviourCmd)
}

func runBehaviour(cmd *cobra.Command, args []string) error {
	if behaviourDate != "" {
		day, err := parseDateFlag(behaviourDate)
		if err != nil {
			return err
		}
		return runCommand("behaviour", "day:"+day.Format(dateFormat), func(client *mcas.Client) ([]map[string]string, error) {
			return client.BehaviourDay(day)
		}, renderBehaviourDayText)
	}
	return runCommand("behaviour", "year", func(client *mcas.Client) (*mcas.BehaviourYear, error) {
		return client.BehaviourYear()
	}, renderBehaviourYearText)
}

func renderBehaviourDayText(w io.Writer, data any) error {
	rows := data.([]map[string]string)
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "No behaviour events recorded.")
		return err
	}
	for _, row := range rows {
		if event, ok := row["Event"]; ok && len(row) == 1 {
			if _, err := fmt.Fprintln(w, "-", event); err != nil {
				return err
			}
			continue
		}
		keys := make([]string, 0, len(row))
		for k := range row {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+": "+row[k])
		}
		if _, err := fmt.Fprintln(w, "-", strings.Join(parts, ", ")); err != nil {
			return err
		}
	}
	return nil
}

func renderBehaviourYearText(w io.Writer, data any) error {
	year := data.(*mcas.BehaviourYear)
	if _, err := fmt.Fprintf(w, "%s: %d points (%d positive, %d negative)\n", year.YearName, year.Points.Total, year.Points.Positive, year.Points.Negative); err != nil {
		return err
	}
	if len(year.Events) == 0 {
		_, err := fmt.Fprintln(w, "No behaviour events recorded.")
		return err
	}
	limit := len(year.Events)
	if limit > 10 {
		limit = 10
	}
	for _, event := range year.Events[:limit] {
		if _, err := fmt.Fprintf(w, "- %s [%s] %s (%+d points)\n", event.Date.Format(dateFormat), event.Type, event.Subject, event.Points); err != nil {
			return err
		}
	}
	return nil
}
