package mcas

import (
	"testing"
	"time"
)

// Fixture HTML fragments mirror tests/conftest.py in the reference
// ha-mychildatschool-mcas repo (fabricated values, no real pupil data).
const loginHTML = `
<html><body><form method="post" action="./MCSParentLogin">
  <input type="hidden" name="__VIEWSTATE" value="abc123" />
  <input type="hidden" name="__VIEWSTATEGENERATOR" value="F50534A4" />
  <input type="hidden" name="__EVENTVALIDATION" value="xyz789" />
  <input name="EmailTextBox" type="text" />
  <input name="PasswordTextBox" type="password" />
</form></body></html>
`

const dashboardHTML = `
<html><body>
  <span id="ctl00_StudentNameLabel">Smith, Alex</span>
  <script>
    var master_studentid = 99999;
    var master_userid = 12345;
    var master_schoolID = "88888";
    var master_schoolName = "Example High School";
  </script>
</body></html>
`

const behaviourHTML = `
<table class='table table-theme'><thead><tr>
  <th>Date</th><th>Class</th><th>Teacher</th><th>Event</th><th>Outcome</th>
</tr></thead><tbody>
<tr><td>07/09/2026</td><td>7X</td><td>Mr A Teacher</td>
    <td><i class='fa fa-times-circle'></i> Example negative event</td><td>SANCT</td></tr>
<tr><td>07/09/2026</td><td>7X</td><td>Mr A Teacher</td>
    <td><i class='fa fa-check-circle'></i> example positive event</td><td>REWARD</td></tr>
<tr><td>07/09/2026</td><td>7X</td><td>Mr A Teacher</td>
    <td><i class='fa fa-star'></i> unknown icon</td><td></td></tr>
</tbody></table>
`

const timetableHTML = `
<html><body><table>
  <tr><th>Monday14th Sep</th><th>Tuesday15th Sep</th></tr>
  <tr>
    <td><div>Tutor</div><div title="Example High School">Example Hig...</div>
        <div title="Tutor Period">Tutor Period</div><div title="7ZZ">7ZZ</div>
        <div title="Ms Example">Ms Example</div></td>
    <td><div>Tutor</div><div title="Example High School">Example Hig...</div>
        <div title="Tutor Period">Tutor Period</div><div title="7ZZ">7ZZ</div>
        <div title="Ms Example">Ms Example</div></td>
  </tr>
  <tr>
    <td><div>1</div><div title="Example High School">Example Hig...</div>
        <div title="Rel. Stud.">Rel. Stud.</div><div title="7y/Re1">7y/Re1</div>
        <div title="Miss Example">Miss Example</div></td>
    <td><div>1</div><div title="Example High School">Example Hig...</div>
        <div title="Drama">Drama</div><div title="7y/Dr1">7y/Dr1</div>
        <div title="Mr Example">Mr Example</div></td>
  </tr>
</table></body></html>
`

func TestHiddenFields(t *testing.T) {
	fields := hiddenFields(loginHTML)
	if fields["__VIEWSTATE"] != "abc123" {
		t.Errorf("__VIEWSTATE = %q, want abc123", fields["__VIEWSTATE"])
	}
	if fields["__EVENTVALIDATION"] != "xyz789" {
		t.Errorf("__EVENTVALIDATION = %q, want xyz789", fields["__EVENTVALIDATION"])
	}
	if _, ok := fields["EmailTextBox"]; ok {
		t.Errorf("EmailTextBox should not be scraped as a hidden field")
	}
}

func TestReadDashboardContext(t *testing.T) {
	ctx := readDashboardContext(dashboardHTML)
	if ctx.StudentID != 99999 {
		t.Errorf("StudentID = %d, want 99999", ctx.StudentID)
	}
	if ctx.SchoolID != 88888 {
		t.Errorf("SchoolID = %d, want 88888", ctx.SchoolID)
	}
	if ctx.SchoolName != "Example High School" {
		t.Errorf("SchoolName = %q, want Example High School", ctx.SchoolName)
	}
	if ctx.StudentName != "Alex Smith" {
		t.Errorf("StudentName = %q, want Alex Smith (Surname, Forename reordered)", ctx.StudentName)
	}
	if ctx.UserID != 12345 {
		t.Errorf("UserID = %d, want 12345", ctx.UserID)
	}
}

func TestExtractLinksTrimsTrailingPunctuation(t *testing.T) {
	tests := []struct {
		body string
		want []string
	}{
		{"See https://forms.office.com/abc for details.", []string{"https://forms.office.com/abc"}},
		{"No links here.", nil},
		{"Two links: https://a.example (info) and www.b.example, thanks.", []string{"https://a.example", "www.b.example"}},
	}
	for _, tt := range tests {
		got := extractLinks(tt.body)
		if len(got) != len(tt.want) {
			t.Errorf("extractLinks(%q) = %v, want %v", tt.body, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("extractLinks(%q)[%d] = %q, want %q", tt.body, i, got[i], tt.want[i])
			}
		}
	}
}

func TestParseMessageDateHandlesVariableFractionalSeconds(t *testing.T) {
	tests := []string{
		"2026-09-24T16:16:33.42",
		"2026-09-24T16:16:33.9",
		"2026-09-24T16:16:33.999",
		"2026-09-24T16:16:33",
	}
	for _, s := range tests {
		got := parseMessageDate(s)
		if got.IsZero() {
			t.Errorf("parseMessageDate(%q) is zero, want a parsed time", s)
		}
		if got.Year() != 2026 || got.Month() != 9 || got.Day() != 24 {
			t.Errorf("parseMessageDate(%q) = %v, want 2026-09-24", s, got)
		}
	}
}

func TestParseBehaviourHTML(t *testing.T) {
	events := parseBehaviourHTML(behaviourHTML)
	want := []BehaviourDayEvent{
		{Date: "2026-09-07", Class: "7X", Teacher: "Mr A Teacher", Description: "Example negative event", Outcome: "SANCT", Type: "Negative"},
		{Date: "2026-09-07", Class: "7X", Teacher: "Mr A Teacher", Description: "example positive event", Outcome: "REWARD", Type: "Positive"},
		{Date: "2026-09-07", Class: "7X", Teacher: "Mr A Teacher", Description: "unknown icon"},
	}
	if len(events) != len(want) {
		t.Fatalf("got %d events, want %d", len(events), len(want))
	}
	for i := range want {
		if events[i] != want[i] {
			t.Errorf("events[%d] = %+v, want %+v", i, events[i], want[i])
		}
	}
}

func TestParseBehaviourHTMLHandlesJunk(t *testing.T) {
	for _, v := range []string{"", "<table></table>"} {
		if got := parseBehaviourHTML(v); len(got) != 0 {
			t.Errorf("parseBehaviourHTML(%q) = %v, want empty", v, got)
		}
	}
}

func TestParseTimetableHTML(t *testing.T) {
	today := time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC)
	lessons := parseTimetableHTML(timetableHTML, today)
	if len(lessons) != 4 {
		t.Fatalf("got %d lessons, want 4", len(lessons))
	}
	var tutor []Lesson
	for _, l := range lessons {
		if l.Period == "Tutor" {
			tutor = append(tutor, l)
		}
	}
	if len(tutor) == 0 {
		t.Fatal("no Tutor period lessons found")
	}
	if tutor[0].Subject != "Tutor Period" {
		t.Errorf("Subject = %q, want Tutor Period", tutor[0].Subject)
	}
	if tutor[0].Teacher != "Ms Example" {
		t.Errorf("Teacher = %q, want Ms Example", tutor[0].Teacher)
	}
	if tutor[0].Day != "Monday" {
		t.Errorf("Day = %q, want Monday", tutor[0].Day)
	}

	subjects := map[string]bool{}
	for _, l := range lessons {
		subjects[l.Subject] = true
	}
	for _, want := range []string{"Tutor Period", "Rel. Stud.", "Drama"} {
		if !subjects[want] {
			t.Errorf("missing lesson subject %q in %v", want, subjects)
		}
	}
}

func TestParseDayHeader(t *testing.T) {
	today := time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		header  string
		wantDay string
	}{
		{"Monday14th Sep", "Monday"},
		{"Friday1st Nov", "Friday"},
		{"nonsense", "nonsense"},
	}
	for _, tt := range tests {
		day, _ := parseDayHeader(tt.header, today)
		if day != tt.wantDay {
			t.Errorf("parseDayHeader(%q) day = %q, want %q", tt.header, day, tt.wantDay)
		}
	}
}

func TestParseDinnerBalance(t *testing.T) {
	tests := []struct {
		html string
		want float64
		ok   bool
	}{
		{"<span>Credit Balance Summary : £ 12.34</span>", 12.34, true},
		{"<span>£ -5.00</span>", -5.0, true},
		{"<span>£ 1,234.56</span>", 1234.56, true},
		{"<span>no balance published</span>", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseDinnerBalance(tt.html)
		if ok != tt.ok || (ok && got != tt.want) {
			t.Errorf("parseDinnerBalance(%q) = (%v, %v), want (%v, %v)", tt.html, got, ok, tt.want, tt.ok)
		}
	}
}
