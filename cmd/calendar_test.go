package cmd

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dental-dash/my-child-at-school-cli/internal/mcas"
)

// buildDays makes consecutive days from start, one type code per day:
// - school, * weekend, # holiday, $ staff day, ? unknown.
func buildDays(t *testing.T, start string, codes string) []mcas.CalendarDay {
	t.Helper()
	d, err := time.Parse(dateFormat, start)
	if err != nil {
		t.Fatal(err)
	}
	types := map[rune]mcas.DayType{'-': mcas.SchoolDay, '*': mcas.Weekend, '#': mcas.Holiday, '$': mcas.StaffDay, '?': mcas.Unknown}
	var out []mcas.CalendarDay
	for i, c := range codes {
		out = append(out, mcas.CalendarDay{Date: d.AddDate(0, 0, i).Format(dateFormat), Type: types[c]})
	}
	return out
}

func TestRangesOfBridgesWeekends(t *testing.T) {
	// Christmas shape: Mon-Fri holiday, weekend, Mon-Fri holiday, then school.
	// Starts Monday 2026-12-21.
	days := buildDays(t, "2026-12-21", "#####**#####**-")
	got := rangesOf(days, mcas.Holiday)
	want := []dateRange{{"2026-12-21", "2027-01-01"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rangesOf = %v, want %v", got, want)
	}
}

func TestRangesOfEasterShape(t *testing.T) {
	// Fri 2027-03-26 holiday, weekend, Mon-Fri, weekend, Mon-Fri.
	days := buildDays(t, "2027-03-26", "#**#####**#####-")
	got := rangesOf(days, mcas.Holiday)
	want := []dateRange{{"2027-03-26", "2027-04-09"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rangesOf = %v, want %v", got, want)
	}
}

func TestRangesOfNeverBridgesSchoolDaysOrOtherTypes(t *testing.T) {
	// Holiday, school day, holiday; and staff days straight into a holiday.
	days := buildDays(t, "2027-07-14", "#-#$$#")
	if got, want := rangesOf(days, mcas.Holiday), []dateRange{{"2027-07-14", "2027-07-14"}, {"2027-07-16", "2027-07-16"}, {"2027-07-19", "2027-07-19"}}; !reflect.DeepEqual(got, want) {
		// 14 holiday, 15 school, 16 holiday, 17-18 staff, 19 holiday
		t.Errorf("holiday ranges = %v, want %v", got, want)
	}
	if got, want := rangesOf(days, mcas.StaffDay), []dateRange{{"2027-07-17", "2027-07-18"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("staff ranges = %v, want %v", got, want)
	}
}

func TestRenderCalendarText(t *testing.T) {
	days := buildDays(t, "2026-09-01", "$$-#*?")
	cal := &mcas.AcademicCalendar{YearName: "2026/2027", Days: days}
	var buf bytes.Buffer
	if err := renderCalendarText(&buf, cal); err != nil {
		t.Fatal(err)
	}
	want := "2026/2027: 1 school days, 1 holiday days, 2 staff days, 1 unknown days (2026-09-01 to 2026-09-06)\n" +
		"Holidays:\n- 2026-09-04\n" +
		"Staff days:\n- 2026-09-01 to 2026-09-02\n"
	if buf.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
}

func TestCalendarDay(t *testing.T) {
	cal := &mcas.AcademicCalendar{YearName: "2026/2027", Days: buildDays(t, "2026-09-05", "*-")}
	// Add a gap: 2026-09-05 weekend, 2026-09-06 school; then jump.
	cal.Days = append(cal.Days, mcas.CalendarDay{Date: "2026-09-08", Type: mcas.SchoolDay})

	if d, err := calendarDay(cal, "2026-09-05"); err != nil || d.Type != mcas.Weekend {
		t.Errorf("weekend lookup = %+v, %v", d, err)
	}
	if d, err := calendarDay(cal, "2026-09-07"); err != nil || d.Type != mcas.Unknown {
		t.Errorf("gap lookup = %+v, %v, want unknown", d, err)
	}
	_, err := calendarDay(cal, "2027-09-01")
	var apiErr *mcas.APIError
	if !errors.As(err, &apiErr) || !strings.Contains(err.Error(), "2027-09-01 is outside the 2026/2027 academic calendar") {
		t.Errorf("outside lookup error = %v", err)
	}
}

func TestRenderCalendarDayText(t *testing.T) {
	var buf bytes.Buffer
	if err := renderCalendarDayText(&buf, mcas.CalendarDay{Date: "2026-09-07", Type: mcas.SchoolDay}); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "2026-09-07: school day\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
