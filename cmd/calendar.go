package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dental-dash/my-child-at-school-cli/internal/mcas"
)

var calendarDate string

var calendarCmd = &cobra.Command{
	Use:   "calendar",
	Short: "Show the school's academic calendar for the year, or a single day with --date",
	RunE:  runCalendar,
}

func init() {
	calendarCmd.Flags().StringVar(&calendarDate, "date", "", "show the day type for a single date, YYYY-MM-DD, instead of the year summary")
	rootCmd.AddCommand(calendarCmd)
}

func runCalendar(cmd *cobra.Command, args []string) error {
	fetch := func(client *mcas.Client) (*mcas.AcademicCalendar, error) {
		return client.Calendar()
	}
	if calendarDate == "" {
		return runCommand("calendar", "year", fetch, renderCalendarText)
	}
	day, err := parseDateFlag(calendarDate)
	if err != nil {
		return err
	}
	date := day.Format(dateFormat)
	return runCommandThen("calendar", "year", fetch, func(cal *mcas.AcademicCalendar) (mcas.CalendarDay, error) {
		return calendarDay(cal, date)
	}, renderCalendarDayText)
}

// calendarDay looks up one date. A date outside the calendar's span is an
// error; a gap inside it is reported as unknown.
func calendarDay(cal *mcas.AcademicCalendar, date string) (mcas.CalendarDay, error) {
	if len(cal.Days) == 0 || date < cal.Days[0].Date || date > cal.Days[len(cal.Days)-1].Date {
		return mcas.CalendarDay{}, &mcas.APIError{Message: fmt.Sprintf("%s is outside the %s academic calendar", date, cal.YearName)}
	}
	for _, d := range cal.Days {
		if d.Date == date {
			return d, nil
		}
	}
	return mcas.CalendarDay{Date: date, Type: mcas.Unknown}, nil
}

// dateRange is an inclusive run of ISO dates.
type dateRange struct{ Start, End string }

func (r dateRange) String() string {
	if r.Start == r.End {
		return r.Start
	}
	return r.Start + " to " + r.End
}

// rangesOf merges consecutive days of type t into ranges. Weekend days
// between two days of type t are bridged (MCAS marks weekends inside a
// holiday as weekend, not holiday), but a school day or a day of another
// type ends the range.
func rangesOf(days []mcas.CalendarDay, t mcas.DayType) []dateRange {
	var out []dateRange
	open := false
	for _, d := range days {
		switch {
		case d.Type == t:
			if open {
				out[len(out)-1].End = d.Date
			} else {
				out = append(out, dateRange{Start: d.Date, End: d.Date})
				open = true
			}
		case d.Type == mcas.Weekend:
		default:
			open = false
		}
	}
	return out
}

func renderCalendarText(w io.Writer, data any) error {
	cal := data.(*mcas.AcademicCalendar)
	counts := map[mcas.DayType]int{}
	for _, d := range cal.Days {
		counts[d.Type]++
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d school days, %d holiday days, %d staff days", cal.YearName, counts[mcas.SchoolDay], counts[mcas.Holiday], counts[mcas.StaffDay])
	if n := counts[mcas.Unknown]; n > 0 {
		fmt.Fprintf(&b, ", %d unknown days", n)
	}
	if len(cal.Days) > 0 {
		fmt.Fprintf(&b, " (%s to %s)", cal.Days[0].Date, cal.Days[len(cal.Days)-1].Date)
	}
	b.WriteString("\n")
	for _, section := range []struct {
		title string
		t     mcas.DayType
	}{{"Holidays:", mcas.Holiday}, {"Staff days:", mcas.StaffDay}} {
		ranges := rangesOf(cal.Days, section.t)
		if len(ranges) == 0 {
			continue
		}
		b.WriteString(section.title + "\n")
		for _, r := range ranges {
			b.WriteString("- " + r.String() + "\n")
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func renderCalendarDayText(w io.Writer, data any) error {
	day := data.(mcas.CalendarDay)
	_, err := fmt.Fprintf(w, "%s: %s\n", day.Date, strings.ReplaceAll(string(day.Type), "_", " "))
	return err
}
