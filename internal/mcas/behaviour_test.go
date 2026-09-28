package mcas

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestUnquoteProxyString(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"quoted", `"<b>a \"q\"</b>"`, `<b>a "q"</b>`},
		{"bare", `<b>a</b>`, `<b>a</b>`},
		{"empty", ``, ``},
	}
	for _, tc := range tests {
		got, err := unquoteProxyString(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("%s: unquoteProxyString(%q) = %q, %v; want %q", tc.name, tc.in, got, err, tc.want)
		}
	}
	if _, err := unquoteProxyString(`"unterminated`); err == nil {
		t.Error("unterminated quote: want an error")
	}
}

// The eventstable "d" is a JSON-quoted string wrapping the HTML fragment.
func TestBehaviourDayUnquotesFragmentAndTypesRows(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		quoted, _ := json.Marshal(behaviourHTML)
		body, _ := json.Marshal(map[string]string{"d": string(quoted)})
		w.Write(body)
	})
	client.session = Session{StudentID: 99999, YearID: 40000}

	rows, err := client.BehaviourDay(time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("BehaviourDay() error = %v", err)
	}
	if len(rows) != 3 || rows[0].Type != "Negative" || rows[1].Type != "Positive" || rows[2].Type != "" {
		t.Errorf("rows = %+v, want Negative, Positive, untyped", rows)
	}
}

func TestBehaviourDayEmptyIsNotNil(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"d": ""}`))
	})
	client.session = Session{StudentID: 99999, YearID: 40000}
	rows, err := client.BehaviourDay(time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC))
	if err != nil || rows == nil || len(rows) != 0 {
		t.Errorf("BehaviourDay() = %v, %v; want empty non-nil slice", rows, err)
	}
}

// Year rows with SubjectID -1 match no subject and stay blank; negative
// all-time totals are reported as absolute values.
func TestBehaviourYearUnknownSubjectAndAbsoluteAllTimeNegative(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"d": "{\"Table\":[` +
			`{\"EventRecordID\":100001,\"EventDate\":\"2026-09-07T12:57:00\",\"EventType\":\"Negative\",\"Adjustment\":-1,\"SubjectID\":-1},` +
			`{\"EventRecordID\":100002,\"EventDate\":\"2026-09-07T12:58:00\",\"EventType\":\"Positive\",\"Adjustment\":1,\"SubjectID\":-1}` +
			`],\"Table3\":[{\"SubjectID\":1,\"SubjectName\":\"Music\"}],\"Table4\":[` +
			`{\"ShowTotalPointsAllTime\":\"-39\",\"PositivePointsAllTime\":\"1\",\"NegativePointsAllTime\":\"-40\"}]}"}`))
	})
	client.session = Session{StudentID: 99999, YearID: 40000}

	year, err := client.BehaviourYear()
	if err != nil {
		t.Fatalf("BehaviourYear() error = %v", err)
	}
	if year.Events[0].ID != 100002 || year.Events[0].Subject != "" || year.Events[0].Points != 1 {
		t.Errorf("events[0] = %+v", year.Events[0])
	}
	if year.Events[1].Points != -1 {
		t.Errorf("per-event points = %d, want signed -1", year.Events[1].Points)
	}
	if got := year.Points.AllTimeNegative; got == nil || *got != 40 {
		t.Errorf("AllTimeNegative = %v, want 40", got)
	}
}

func day7(h, m int) time.Time { return time.Date(2026, 9, 7, h, m, 0, 0, time.UTC) }

func TestApplyDayDetailsPairsByTimeOrder(t *testing.T) {
	// Newest-first, as BehaviourYear returns them.
	events := []BehaviourEvent{
		{ID: 100002, Date: day7(12, 58), Type: "Positive"},
		{ID: 100001, Date: day7(12, 57), Type: "Negative"},
		{ID: 5, Date: day7(12, 57).AddDate(0, 0, 1), Type: "Negative"}, // other day, untouched
	}
	rows := []BehaviourDayEvent{
		{Class: "7X", Teacher: "Mr A Teacher", Description: "neg", Outcome: "SANCT", Type: "Negative"},
		{Class: "7X", Teacher: "Mr A Teacher", Description: "pos", Outcome: "REWARD", Type: "Positive"},
	}
	if err := ApplyDayDetails(events, "2026-09-07", rows); err != nil {
		t.Fatalf("ApplyDayDetails() error = %v", err)
	}
	if events[1].Description != "neg" || events[0].Description != "pos" || events[0].Outcome != "REWARD" {
		t.Errorf("events = %+v", events)
	}
	if events[2].Description != "" {
		t.Errorf("other-day event was enriched: %+v", events[2])
	}
}

func TestApplyDayDetailsSameTimestampOrdersByID(t *testing.T) {
	events := []BehaviourEvent{
		{ID: 2, Date: day7(9, 0), Type: "Positive"},
		{ID: 1, Date: day7(9, 0), Type: "Negative"},
	}
	rows := []BehaviourDayEvent{
		{Description: "first", Type: "Negative"},
		{Description: "second", Type: "Positive"},
	}
	if err := ApplyDayDetails(events, "2026-09-07", rows); err != nil {
		t.Fatalf("error = %v", err)
	}
	if events[1].Description != "first" || events[0].Description != "second" {
		t.Errorf("events = %+v", events)
	}
}

func TestApplyDayDetailsLeavesDayBlankOnMismatch(t *testing.T) {
	newEvents := func() []BehaviourEvent {
		return []BehaviourEvent{
			{ID: 1, Date: day7(12, 57), Type: "Negative"},
			{ID: 2, Date: day7(12, 58), Type: "Positive"},
		}
	}
	tests := map[string][]BehaviourDayEvent{
		"count":    {{Description: "only one", Type: "Negative"}},
		"shuffled": {{Description: "a", Type: "Positive"}, {Description: "b", Type: "Negative"}},
	}
	for name, rows := range tests {
		events := newEvents()
		if err := ApplyDayDetails(events, "2026-09-07", rows); err == nil {
			t.Errorf("%s: want an error", name)
		}
		for _, e := range events {
			if e.Description != "" || e.Teacher != "" {
				t.Errorf("%s: event enriched despite mismatch: %+v", name, e)
			}
		}
	}
}

func TestApplyDayDetailsUntypedRowIsWildcard(t *testing.T) {
	events := []BehaviourEvent{{ID: 1, Date: day7(9, 0), Type: "Neutral"}}
	rows := []BehaviourDayEvent{{Description: "note"}}
	if err := ApplyDayDetails(events, "2026-09-07", rows); err != nil || events[0].Description != "note" {
		t.Errorf("err = %v, events = %+v", err, events)
	}
}
