package mcas

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestBehaviourYearComputesPointsAndMapsSubjects mirrors
// test_behaviour_year_computes_points_and_maps_subjects: summing Adjustment
// reproduces the total the portal shows, newest event first, with
// SubjectID resolved to a name.
func TestBehaviourYearComputesPointsAndMapsSubjects(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"d": "{\"Table\":[` +
			`{\"EventDate\":\"2026-09-11T09:00:00\",\"EventType\":\"Positive\",\"Adjustment\":2,\"SubjectID\":1,\"EventRecordID\":10},` +
			`{\"EventDate\":\"2026-09-10T09:00:00\",\"EventType\":\"Neutral\",\"Adjustment\":0,\"SubjectID\":2,\"EventRecordID\":11},` +
			`{\"EventDate\":\"2026-09-09T09:00:00\",\"EventType\":\"Negative\",\"Adjustment\":-1,\"SubjectID\":1,\"EventRecordID\":12}` +
			`],\"Table1\":[` +
			`{\"Day\":\"2026-09-11T00:00:00\",\"DayStatusCode\":\"-\"},` +
			`{\"Day\":\"2026-09-12T00:00:00\",\"DayStatusCode\":\"*\"},` +
			`{\"Day\":\"2026-09-14T00:00:00\",\"DayStatusCode\":\"#\"},` +
			`{\"Day\":\"2026-09-15T00:00:00\",\"DayStatusCode\":\"$\"}` +
			`],\"Table2\":[{\"YearName\":\"2026/2027\"}],\"Table3\":[` +
			`{\"SubjectID\":1,\"SubjectName\":\"Music\"},{\"SubjectID\":2,\"SubjectName\":\"Maths\"}` +
			`],\"Table4\":{}}"}`))
	})
	client.session = Session{StudentID: 99999, YearID: 40000}

	year, err := client.BehaviourYear()
	if err != nil {
		t.Fatalf("BehaviourYear() error = %v", err)
	}
	if year.YearName != "2026/2027" {
		t.Errorf("YearName = %q, want 2026/2027", year.YearName)
	}
	if year.Points.Positive != 2 {
		t.Errorf("Positive = %d, want 2", year.Points.Positive)
	}
	if year.Points.Negative != 1 {
		t.Errorf("Negative = %d, want 1", year.Points.Negative)
	}
	if year.Points.Total != 1 {
		t.Errorf("Total = %d, want 1", year.Points.Total)
	}
	if len(year.Events) != 3 {
		t.Fatalf("got %d events, want 3", len(year.Events))
	}
	if year.Events[0].Subject != "Music" {
		t.Errorf("newest event subject = %q, want Music", year.Events[0].Subject)
	}
	wantCalendar := map[string]DayType{
		"2026-09-11": SchoolDay,
		"2026-09-12": Weekend,
		"2026-09-14": Holiday,
		"2026-09-15": StaffDay,
	}
	for date, want := range wantCalendar {
		if got := year.Calendar[date]; got != want {
			t.Errorf("Calendar[%q] = %q, want %q", date, got, want)
		}
	}
}

// TestAllTimeTotalsToleratesNA mirrors test_all_time_totals_tolerate_na:
// schools can hide a figure, in which case MCAS returns the string "N/A".
func TestAllTimeTotalsToleratesNA(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"d": "{\"Table\":[],\"Table3\":[],\"Table4\":[` +
			`{\"ShowTotalPointsAllTime\":\"12\",\"PositivePointsAllTime\":\"12\",\"NegativePointsAllTime\":\"N/A\"}` +
			`]}"}`))
	})
	client.session = Session{StudentID: 99999, YearID: 40000}

	year, err := client.BehaviourYear()
	if err != nil {
		t.Fatalf("BehaviourYear() error = %v", err)
	}
	if year.Points.AllTimeTotal == nil || *year.Points.AllTimeTotal != 12 {
		t.Errorf("AllTimeTotal = %v, want 12", year.Points.AllTimeTotal)
	}
	if year.Points.AllTimeNegative != nil {
		t.Errorf("AllTimeNegative = %v, want nil", *year.Points.AllTimeNegative)
	}
}

// TestModulesParsesStringBooleans mirrors test_modules_parses_string_booleans:
// flags come back as the strings "True"/"False" with inconsistent casing.
func TestModulesParsesStringBooleans(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"d": "{\"Table\":[` +
			`{\"KeyName\":\"MCASAttendanceModuleEnabled\",\"KeyValue\":\"True\"},` +
			`{\"KeyName\":\"MCASBehaviourModuleEnabled\",\"KeyValue\":\"true\"},` +
			`{\"KeyName\":\"MCASDinnerMoneyModule_EnableDinnerMoneyModule\",\"KeyValue\":\"False\"},` +
			`{\"KeyName\":\"MCASClubsModuleEnabled\",\"KeyValue\":\"false\"}` +
			`]}"}`))
	})

	modules, err := client.Modules()
	if err != nil {
		t.Fatalf("Modules() error = %v", err)
	}
	if !modules.Attendance || !modules.Behaviour {
		t.Errorf("modules = %+v, want Attendance and Behaviour true", modules)
	}
	if modules.Dinner || modules.Clubs || modules.Trips {
		t.Errorf("modules = %+v, want Dinner, Clubs and Trips false", modules)
	}
}

// TestDinnerBalanceScrapesTheWidget mirrors test_dinner_balance_scrapes_the_widget.
func TestDinnerBalanceScrapesTheWidget(t *testing.T) {
	tests := []struct {
		html string
		want *float64
	}{
		{"<span>Credit Balance Summary : £ 12.34</span>", ptr(12.34)},
		{"<span>£ -5.00</span>", ptr(-5.0)},
		{"<span>£ 1,234.56</span>", ptr(1234.56)},
		{"<span>no balance published</span>", nil},
	}
	for _, tt := range tests {
		encoded, err := json.Marshal(tt.html)
		if err != nil {
			t.Fatalf("json.Marshal(%q) error = %v", tt.html, err)
		}
		client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"d": ` + string(encoded) + `}`))
		})
		client.session = Session{StudentID: 99999}
		balance, err := client.DinnerBalance()
		if err != nil {
			t.Fatalf("DinnerBalance() error = %v", err)
		}
		if tt.want == nil {
			if balance != nil {
				t.Errorf("DinnerBalance() = %+v, want nil", balance)
			}
			continue
		}
		if balance == nil || balance.Amount != *tt.want {
			t.Errorf("DinnerBalance() = %+v, want Amount=%v", balance, *tt.want)
		}
	}
}

func ptr[T any](v T) *T { return &v }
