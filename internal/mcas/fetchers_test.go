package mcas

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
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
}

// TestCalendarDecodesAndSortsDays covers the calendar split out of
// BehaviourYear: DayStatusCode decoding, date truncation, sorting, and an
// unmapped code falling through to Unknown.
func TestCalendarDecodesAndSortsDays(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"d": "{\"Table\":[],\"Table1\":[` +
			`{\"Day\":\"2026-09-15T00:00:00\",\"DayStatusCode\":\"$\"},` +
			`{\"Day\":\"2026-09-11T00:00:00\",\"DayStatusCode\":\"-\"},` +
			`{\"Day\":\"2026-09-12T00:00:00\",\"DayStatusCode\":\"*\"},` +
			`{\"Day\":\"2026-09-14T00:00:00\",\"DayStatusCode\":\"#\"},` +
			`{\"Day\":\"2026-09-16T00:00:00\",\"DayStatusCode\":\"?\"}` +
			`],\"Table2\":[{\"YearName\":\"2026/2027\"}]}"}`))
	})
	client.session = Session{StudentID: 99999, YearID: 40000}

	cal, err := client.Calendar()
	if err != nil {
		t.Fatalf("Calendar() error = %v", err)
	}
	if cal.YearName != "2026/2027" {
		t.Errorf("YearName = %q, want 2026/2027", cal.YearName)
	}
	want := []CalendarDay{
		{"2026-09-11", SchoolDay},
		{"2026-09-12", Weekend},
		{"2026-09-14", Holiday},
		{"2026-09-15", StaffDay},
		{"2026-09-16", Unknown},
	}
	if len(cal.Days) != len(want) {
		t.Fatalf("got %d days, want %d", len(cal.Days), len(want))
	}
	for i, d := range want {
		if cal.Days[i] != d {
			t.Errorf("Days[%d] = %+v, want %+v", i, cal.Days[i], d)
		}
	}
}

// TestCalendarErrorsWhenEmpty: no Table1 rows means the Behaviour module
// isn't providing a calendar, which is reported rather than returned empty.
func TestCalendarErrorsWhenEmpty(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"d": "{\"Table\":[],\"Table1\":[]}"}`))
	})
	client.session = Session{StudentID: 99999, YearID: 40000}

	_, err := client.Calendar()
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Calendar() error = %v, want *APIError", err)
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

// TestConversationsJoinsAttachmentsAndExtractsLinks mirrors the live-capture
// schema recorded in the issue analysis: attachments arrive as a flat list
// keyed by MessageID and must be joined onto their message, and bare URLs
// in the plain-text body become the Links field.
func TestConversationsJoinsAttachmentsAndExtractsLinks(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"d": "{\"Conversations\":[` +
			`{\"RecipientID\":508,\"RecipientName\":\"Mrs Smith\",\"UnreadCount\":1,\"PublishedDocumentID\":null,\"Messages\":[` +
			`{\"MessageID\":1001,\"Subject\":\"Homework\",\"Message\":\"See https://forms.office.com/abc for the form.\",\"MessageDate\":\"2026-09-24T16:16:33.42\",\"Mode\":0,\"IsRead\":false}` +
			`]}` +
			`],\"MessageAttachments\":[` +
			`{\"AttachmentID\":52,\"FileName\":\"letter.pdf\",\"MessageID\":1001}` +
			`]}"}`))
	})
	client.session = Session{StudentID: 99999, UserID: 12345}

	conversations, err := client.Conversations()
	if err != nil {
		t.Fatalf("Conversations() error = %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("got %d conversations, want 1", len(conversations))
	}
	conv := conversations[0]
	if conv.RecipientID != 508 || conv.RecipientName != "Mrs Smith" || conv.UnreadCount != 1 {
		t.Errorf("conversation = %+v, want RecipientID=508 RecipientName=Mrs Smith UnreadCount=1", conv)
	}
	if len(conv.Messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(conv.Messages))
	}
	msg := conv.Messages[0]
	if msg.Sent {
		t.Errorf("msg.Sent = true, want false for Mode 0")
	}
	if msg.Read {
		t.Errorf("msg.Read = true, want false for IsRead:false")
	}
	if len(msg.Links) != 1 || msg.Links[0] != "https://forms.office.com/abc" {
		t.Errorf("msg.Links = %v, want [https://forms.office.com/abc]", msg.Links)
	}
	if len(msg.Attachments) != 1 || msg.Attachments[0].ID != 52 || msg.Attachments[0].FileName != "letter.pdf" {
		t.Errorf("msg.Attachments = %+v, want one attachment id=52 name=letter.pdf", msg.Attachments)
	}
}

// TestFlattenConversationsOrdersNewestFirst mirrors the address-by-message-id
// redesign: flattening must keep every message with its sender inlined,
// newest first, breaking a tie on Date by the higher message ID - MCAS
// doesn't guarantee order within or across threads.
func TestFlattenConversationsOrdersNewestFirst(t *testing.T) {
	sameDate := time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)
	conversations := []Conversation{
		{
			RecipientID:   508,
			RecipientName: "Mrs S Patel",
			Messages: []Message{
				{ID: 100, Subject: "Older", Date: time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)},
				{ID: 300, Subject: "Tie low", Date: sameDate},
			},
		},
		{
			RecipientID:   1,
			RecipientName: "",
			Messages: []Message{
				{ID: 400, Subject: "Newest", Date: time.Date(2026, 9, 24, 16, 16, 0, 0, time.UTC)},
				{ID: 301, Subject: "Tie high", Date: sameDate},
			},
		},
	}

	messages := FlattenConversations(conversations)
	if len(messages) != 4 {
		t.Fatalf("got %d messages, want 4", len(messages))
	}

	wantOrder := []int{400, 301, 300, 100}
	for i, id := range wantOrder {
		if messages[i].ID != id {
			t.Errorf("messages[%d].ID = %d, want %d (order = %v)", i, messages[i].ID, id, idsOf(messages))
		}
	}

	if messages[3].RecipientID != 508 || messages[3].RecipientName != "Mrs S Patel" {
		t.Errorf("messages[3] = %+v, want RecipientID=508 RecipientName=Mrs S Patel", messages[3])
	}
	if messages[0].RecipientID != 1 || messages[0].RecipientName != "" {
		t.Errorf("messages[0] = %+v, want RecipientID=1 RecipientName=\"\"", messages[0])
	}
}

func idsOf(messages []InboxMessage) []int {
	ids := make([]int, len(messages))
	for i, m := range messages {
		ids[i] = m.ID
	}
	return ids
}

// TestFindMessageLocatesAcrossThreads mirrors "open a single message":
// FindMessage must locate a message regardless of which thread holds it,
// and return an APIError with a stable message when the id isn't found.
func TestFindMessageLocatesAcrossThreads(t *testing.T) {
	conversations := []Conversation{
		{RecipientID: 508, RecipientName: "Mrs S Patel", Messages: []Message{{ID: 1001, Subject: "Homework"}}},
		{RecipientID: 462, RecipientName: "Mrs B Prajapati", Messages: []Message{{ID: 2002, Subject: "Trip"}}},
	}

	msg, err := FindMessage(conversations, 2002)
	if err != nil {
		t.Fatalf("FindMessage(2002) error = %v", err)
	}
	if msg.RecipientID != 462 || msg.Subject != "Trip" {
		t.Errorf("FindMessage(2002) = %+v, want RecipientID=462 Subject=Trip", msg)
	}

	_, err = FindMessage(conversations, 999)
	if err == nil {
		t.Fatal("FindMessage(999) error = nil, want an error for an unknown id")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("FindMessage(999) error = %T, want *APIError", err)
	}
	if apiErr.Message != "no message found with id 999" {
		t.Errorf("FindMessage(999) error message = %q, want %q", apiErr.Message, "no message found with id 999")
	}
}

// TestMessageAttachmentDataDecodesBase64 mirrors the confirmed download
// mechanics: the proxy's "d" wrapper carries a JSON-encoded string whose
// value is the base64 of the raw file - i.e. the base64 is encoded twice.
func TestMessageAttachmentDataDecodesBase64(t *testing.T) {
	want := []byte("%PDF-1.7 fake pdf bytes")
	inner, err := json.Marshal(base64.StdEncoding.EncodeToString(want))
	if err != nil {
		t.Fatalf("json.Marshal error = %v", err)
	}
	outer, err := json.Marshal(string(inner))
	if err != nil {
		t.Fatalf("json.Marshal error = %v", err)
	}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"d": ` + string(outer) + `}`))
	})
	client.session = Session{StudentID: 99999, UserID: 12345}

	got, err := client.MessageAttachmentData(1001, 52)
	if err != nil {
		t.Fatalf("MessageAttachmentData() error = %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("MessageAttachmentData() = %q, want %q", got, want)
	}
}

// TestMessageAttachmentDataAcceptsBareBase64 keeps the single-encoded shape
// working as a fallback, in case a school's proxy doesn't double-encode.
func TestMessageAttachmentDataAcceptsBareBase64(t *testing.T) {
	want := []byte("%PDF-1.7 fake pdf bytes")
	encoded, err := json.Marshal(base64.StdEncoding.EncodeToString(want))
	if err != nil {
		t.Fatalf("json.Marshal error = %v", err)
	}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"d": ` + string(encoded) + `}`))
	})
	client.session = Session{StudentID: 99999, UserID: 12345}

	got, err := client.MessageAttachmentData(1001, 52)
	if err != nil {
		t.Fatalf("MessageAttachmentData() error = %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("MessageAttachmentData() = %q, want %q", got, want)
	}
}

// TestMessageAttachmentDataInvalidQuotedBase64 ensures a quoted payload that
// doesn't hold valid base64 surfaces as an *APIError, not a panic or a
// silently wrong result.
func TestMessageAttachmentDataInvalidQuotedBase64(t *testing.T) {
	outer, err := json.Marshal(`"not valid base64!!"`)
	if err != nil {
		t.Fatalf("json.Marshal error = %v", err)
	}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"d": ` + string(outer) + `}`))
	})
	client.session = Session{StudentID: 99999, UserID: 12345}

	_, err = client.MessageAttachmentData(1001, 52)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("MessageAttachmentData() error = %T, want *APIError", err)
	}
}

// TestMessageAttachmentDataNotFound mirrors the proxy's "Error 404" sentinel
// for an unknown attachment id, which client.get already turns into "".
func TestMessageAttachmentDataNotFound(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"d": "Error 404 - Requested API call reference Not Found."}`))
	})
	client.session = Session{StudentID: 99999, UserID: 12345}

	if _, err := client.MessageAttachmentData(1001, 999); err == nil {
		t.Error("MessageAttachmentData() error = nil, want an error for a missing attachment")
	}
}
