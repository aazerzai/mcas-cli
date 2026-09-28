package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dental-dash/my-child-at-school-cli/internal/mcas"
)

func ev(id int, day, clock, typ string, points int) mcas.BehaviourEvent {
	d, _ := time.Parse("2006-01-02 15:04", day+" "+clock)
	return mcas.BehaviourEvent{ID: id, Date: d, Type: typ, Points: points}
}

func TestFormatBehaviourEvent(t *testing.T) {
	full := ev(2, "2026-09-07", "12:58", "Positive", 1)
	full.Class, full.Teacher, full.Description, full.Outcome = "7X", "Mr A Teacher", "example positive event", "REWARD"
	withSubject := full
	withSubject.Subject = "Spanish"
	bare := ev(1, "2026-09-07", "12:57", "Negative", -1)
	bareSubject := bare
	bareSubject.Subject = "Spanish"
	dateOnly := ev(3, "2026-09-08", "00:00", "Negative", -1)

	tests := []struct {
		name string
		in   mcas.BehaviourEvent
		want string
	}{
		{"class fallback", full, "- 2026-09-07 12:58 [Positive +1] 7X · Mr A Teacher · example positive event (REWARD)"},
		{"subject wins", withSubject, "- 2026-09-07 12:58 [Positive +1] Spanish · Mr A Teacher · example positive event (REWARD)"},
		{"unenriched", bare, "- 2026-09-07 12:57 [Negative -1]"},
		{"unenriched subject", bareSubject, "- 2026-09-07 12:57 [Negative -1] Spanish"},
		{"no time", dateOnly, "- 2026-09-08 [Negative -1]"},
	}
	for _, tc := range tests {
		got := formatBehaviourEvent(tc.in)
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
		if strings.Contains(got, "  ") {
			t.Errorf("%s: %q has a double space", tc.name, got)
		}
	}
}

func TestSelectBehaviourEvents(t *testing.T) {
	events := []mcas.BehaviourEvent{
		ev(4, "2026-09-09", "10:00", "Positive", 1),
		ev(3, "2026-09-08", "10:00", "Positive", 1),
		ev(2, "2026-09-07", "12:58", "Positive", 1),
		ev(1, "2026-09-07", "12:57", "Negative", -1),
	}
	if got := selectBehaviourEvents(events, "", 0); len(got) != 4 {
		t.Errorf("all: got %d", len(got))
	}
	if got := selectBehaviourEvents(events, "", 3); len(got) != 3 || got[0].ID != 4 || got[2].ID != 2 {
		t.Errorf("limit 3: got %+v", got)
	}
	if got := selectBehaviourEvents(events, "2026-09-07", 0); len(got) != 2 {
		t.Errorf("day: got %+v", got)
	}
	if got := selectBehaviourEvents(events, "2030-01-01", 0); got == nil || len(got) != 0 {
		t.Errorf("empty day: got %#v, want empty non-nil", got)
	}
}

func TestEnrichBehaviourEvents(t *testing.T) {
	events := []mcas.BehaviourEvent{
		ev(6, "2026-09-09", "10:00", "Positive", 1),  // day fetch fails
		ev(4, "2026-09-08", "10:00", "Negative", -1), // types shuffled
		ev(3, "2026-09-07", "12:58", "Positive", 1),
		ev(2, "2026-09-07", "12:57", "Negative", -1),
	}
	fetch := func(date string) ([]mcas.BehaviourDayEvent, error) {
		switch date {
		case "2026-09-09":
			return nil, errors.New("HTTP 500")
		case "2026-09-08":
			return []mcas.BehaviourDayEvent{{Description: "wrong", Type: "Positive"}}, nil
		}
		return []mcas.BehaviourDayEvent{
			{Teacher: "Mr A Teacher", Class: "7X", Description: "neg", Type: "Negative"},
			{Teacher: "Mr A Teacher", Class: "7X", Description: "pos", Type: "Positive"},
		}, nil
	}
	var warnings []string
	if err := enrichBehaviourEvents(events, fetch, func(m string) { warnings = append(warnings, m) }); err != nil {
		t.Fatalf("error = %v", err)
	}
	if events[3].Description != "neg" || events[2].Description != "pos" {
		t.Errorf("good day not enriched: %+v", events)
	}
	if events[0].Description != "" || events[1].Description != "" {
		t.Errorf("failed/shuffled days must stay blank: %+v", events)
	}
	if len(warnings) != 2 || !strings.Contains(warnings[0], "2026-09-09") || !strings.Contains(warnings[1], "2026-09-08") {
		t.Errorf("warnings = %q", warnings)
	}
}

func TestEnrichBehaviourEventsAuthErrorIsFatal(t *testing.T) {
	events := []mcas.BehaviourEvent{ev(1, "2026-09-07", "12:57", "Negative", -1)}
	err := enrichBehaviourEvents(events, func(string) ([]mcas.BehaviourDayEvent, error) {
		return nil, &mcas.AuthError{Message: "Session expired"}
	}, func(string) { t.Error("unexpected warning") })
	var authErr *mcas.AuthError
	if !errors.As(err, &authErr) {
		t.Errorf("error = %v, want AuthError", err)
	}
}

func TestRenderBehaviourDayTextEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := renderBehaviourDayText(&buf, []mcas.BehaviourEvent{}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "No behaviour events recorded.\n" {
		t.Errorf("got %q", buf.String())
	}
}
