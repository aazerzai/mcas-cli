package mcas

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// EnsureSession re-logs in if no session has been established yet.
func (c *Client) EnsureSession() error { return c.ensureSession() }

// LoadYearID fetches the current academic YearID, needed by the behaviour
// endpoints, and caches it on the session.
func (c *Client) LoadYearID() (int, error) {
	raw, err := c.get(fmt.Sprintf(epStudentYears, c.session.StudentID))
	if err != nil {
		return 0, err
	}
	var payload struct {
		Table []struct {
			YearID int `json:"YearID"`
		} `json:"Table"`
	}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &payload)
	}
	if len(payload.Table) > 0 {
		c.session.YearID = payload.Table[0].YearID
	}
	return c.session.YearID, nil
}

type attendancePeriodRow struct {
	PeriodName      string `json:"PeriodName"`
	MarkSign        string `json:"MarkSign"`
	MarkMeaning     string `json:"MarkMeaning"`
	MarkDescription string `json:"MarkDescription"`
	SubjectName     string `json:"SubjectName"`
}

// Attendance fetches the registration marks for one day. MCAS serves a
// single day per call.
func (c *Client) Attendance(day time.Time) (*AttendanceDay, error) {
	raw, err := c.get(fmt.Sprintf(epAttendance, c.session.StudentID, day.Year(), int(day.Month()), day.Day()))
	if err != nil {
		return nil, err
	}
	result := &AttendanceDay{Day: day}
	if raw == "" {
		return result, nil
	}
	var payload struct {
		Table []attendancePeriodRow `json:"Table"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return result, nil
	}
	result.Periods = make([]AttendancePeriod, 0, len(payload.Table))
	for _, row := range payload.Table {
		result.Periods = append(result.Periods, AttendancePeriod{
			PeriodName:      row.PeriodName,
			MarkSign:        row.MarkSign,
			MarkMeaning:     row.MarkMeaning,
			MarkDescription: row.MarkDescription,
			SubjectName:     row.SubjectName,
		})
	}
	return result, nil
}

// BehaviourDay fetches the per-day behaviour table for one day. It is the
// only source of an event's description, teacher, class and outcome; see
// ApplyDayDetails for joining it to the year's events.
func (c *Client) BehaviourDay(day time.Time) ([]BehaviourDayEvent, error) {
	if c.session.YearID == 0 {
		if _, err := c.LoadYearID(); err != nil {
			return nil, err
		}
	}
	raw, err := c.get(fmt.Sprintf(epBehaviour, c.session.StudentID, c.session.YearID, day.Year(), int(day.Month()), day.Day()))
	if err != nil {
		return nil, err
	}
	fragment, err := unquoteProxyString(raw)
	if err != nil {
		return nil, &APIError{Message: "decoding behaviour day: " + err.Error()}
	}
	rows := parseBehaviourHTML(fragment)
	if rows == nil {
		rows = []BehaviourDayEvent{}
	}
	return rows, nil
}

// ApplyDayDetails copies description, teacher, class and outcome from one
// day's table rows onto that day's events in events (matched by date, ISO
// YYYY-MM-DD). The rows carry no ID, so they are paired by position with the
// day's events in time order (ties by ID), with each row's icon type as a
// sanity check. On any mismatch nothing is changed and an error says why:
// blank is better than wrong.
func ApplyDayDetails(events []BehaviourEvent, day string, rows []BehaviourDayEvent) error {
	var idx []int
	for i, e := range events {
		if e.Date.Format("2006-01-02") == day {
			idx = append(idx, i)
		}
	}
	sort.SliceStable(idx, func(a, b int) bool {
		x, y := events[idx[a]], events[idx[b]]
		if !x.Date.Equal(y.Date) {
			return x.Date.Before(y.Date)
		}
		return x.ID < y.ID
	})
	if len(idx) != len(rows) {
		return fmt.Errorf("%d events but %d rows", len(idx), len(rows))
	}
	for k, i := range idx {
		if rows[k].Type != "" && rows[k].Type != events[i].Type {
			return fmt.Errorf("row %d is %s but event %d is %s", k+1, rows[k].Type, events[i].ID, events[i].Type)
		}
	}
	for k, i := range idx {
		events[i].Description = rows[k].Description
		events[i].Teacher = rows[k].Teacher
		events[i].Class = rows[k].Class
		events[i].Outcome = rows[k].Outcome
	}
	return nil
}

// unquoteProxyString undoes the proxy's double JSON encoding: some routes
// return a quoted JSON string in "d" rather than the bare value. Bare input
// is returned unchanged.
func unquoteProxyString(raw string) (string, error) {
	payload := strings.TrimSpace(raw)
	if strings.HasPrefix(payload, `"`) {
		var unquoted string
		if err := json.Unmarshal([]byte(payload), &unquoted); err != nil {
			return "", err
		}
		return unquoted, nil
	}
	return raw, nil
}

type behaviourEventRow struct {
	EventDate     string `json:"EventDate"`
	EventType     string `json:"EventType"`
	Adjustment    int    `json:"Adjustment"`
	SubjectID     int    `json:"SubjectID"`
	EventRecordID int    `json:"EventRecordID"`
}

type calendarRow struct {
	Day           string `json:"Day"`
	DayStatusCode string `json:"DayStatusCode"`
}

type subjectRow struct {
	SubjectID   int    `json:"SubjectID"`
	SubjectName string `json:"SubjectName"`
}

type behaviourDetailPayload struct {
	Table  []behaviourEventRow `json:"Table"`
	Table1 []calendarRow       `json:"Table1"`
	Table2 []struct {
		YearName string `json:"YearName"`
	} `json:"Table2"`
	Table3 []subjectRow    `json:"Table3"`
	Table4 json.RawMessage `json:"Table4"`
}

// behaviourDetail fetches and decodes the year's eventdetails payload, which
// carries both the behaviour data and the academic calendar. It returns nil
// for an empty or undecodable payload.
func (c *Client) behaviourDetail() (*behaviourDetailPayload, error) {
	if c.session.YearID == 0 {
		if _, err := c.LoadYearID(); err != nil {
			return nil, err
		}
	}
	raw, err := c.get(fmt.Sprintf(epBehaviourDetail, c.session.StudentID, c.session.YearID))
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	var payload behaviourDetailPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, nil
	}
	return &payload, nil
}

// Calendar returns the school's academic calendar for the year, sorted by
// date. MCAS only provides it in the behaviour module's eventdetails
// payload, so it is unavailable when the school hasn't enabled Behaviour.
func (c *Client) Calendar() (*AcademicCalendar, error) {
	payload, err := c.behaviourDetail()
	if err != nil {
		return nil, err
	}
	if payload == nil || len(payload.Table1) == 0 {
		return nil, &APIError{Message: "no academic calendar available (the school may not have the Behaviour module enabled)"}
	}
	days := make([]CalendarDay, 0, len(payload.Table1))
	for _, row := range payload.Table1 {
		day := row.Day
		if len(day) > 10 {
			day = day[:10]
		}
		dt, ok := dayStatus[row.DayStatusCode]
		if !ok {
			dt = Unknown
		}
		days = append(days, CalendarDay{Date: day, Type: dt})
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Date < days[j].Date })
	cal := &AcademicCalendar{Days: days}
	if len(payload.Table2) > 0 {
		cal.YearName = payload.Table2[0].YearName
	}
	return cal, nil
}

// BehaviourYear fetches the whole academic year's behaviour data in one
// call: points, events and a subject lookup. Strongly preferred over
// walking eventstable day by day - it's JSON rather than an HTML fragment,
// one request instead of one per day, and the only source of the points
// totals. Summing Adjustment reproduces the "Overall Total Points" figure
// shown on the portal's behaviour page.
func (c *Client) BehaviourYear() (*BehaviourYear, error) {
	if c.session.YearID == 0 {
		if _, err := c.LoadYearID(); err != nil {
			return nil, err
		}
	}
	empty := &BehaviourYear{}
	payload, err := c.behaviourDetail()
	if err != nil {
		return nil, err
	}
	if payload == nil {
		return empty, nil
	}

	subjects := map[int]string{}
	for _, s := range payload.Table3 {
		subjects[s.SubjectID] = s.SubjectName
	}

	events := make([]BehaviourEvent, 0, len(payload.Table))
	var positive, negative int
	for _, row := range payload.Table {
		date, _ := time.Parse("2006-01-02T15:04:05", row.EventDate)
		events = append(events, BehaviourEvent{
			ID:      row.EventRecordID,
			Date:    date,
			Type:    row.EventType,
			Points:  row.Adjustment,
			Subject: subjects[row.SubjectID],
		})
		switch row.EventType {
		case "Positive":
			positive += row.Adjustment
		case "Negative":
			negative += row.Adjustment
		}
	}
	sortEventsNewestFirst(events)

	yearName := ""
	if len(payload.Table2) > 0 {
		yearName = payload.Table2[0].YearName
	}
	totals := firstTotalsRow(payload.Table4)

	return &BehaviourYear{
		YearName: yearName,
		Events:   events,
		Points: BehaviourPoints{
			Total:           positive - absInt(negative),
			Positive:        positive,
			Negative:        absInt(negative),
			AllTimeTotal:    asInt(totals["ShowTotalPointsAllTime"]),
			AllTimePositive: asInt(totals["PositivePointsAllTime"]),
			AllTimeNegative: absPtr(asInt(totals["NegativePointsAllTime"])),
		},
	}, nil
}

func firstTotalsRow(raw json.RawMessage) map[string]any {
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err == nil && len(rows) > 0 {
		return rows[0]
	}
	return map[string]any{}
}

// asInt parses a totals figure that arrives as a string, and is "N/A" when
// the school hides that figure.
func asInt(v any) *int {
	s := strings.TrimSpace(fmt.Sprintf("%v", v))
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &n
}

func absPtr(n *int) *int {
	if n == nil {
		return nil
	}
	v := absInt(*n)
	return &v
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Modules reports which MCAS modules this school has switched on. Schools
// license MCAS modules individually; a command that can only ever report
// zero because the school never enabled a module is worse than saying so
// plainly.
func (c *Client) Modules() (ModuleFlags, error) {
	raw, err := c.get(epConfigurations)
	if err != nil {
		return ModuleFlags{}, err
	}
	config := map[string]string{}
	if raw != "" {
		var payload struct {
			Table []struct {
				KeyName  string `json:"KeyName"`
				KeyValue any    `json:"KeyValue"`
			} `json:"Table"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			for _, row := range payload.Table {
				config[row.KeyName] = fmt.Sprintf("%v", row.KeyValue)
			}
		}
	}
	enabled := func(key string) bool {
		return strings.EqualFold(strings.TrimSpace(config[moduleFlags[key]]), "true")
	}
	return ModuleFlags{
		Attendance: enabled("attendance"),
		Behaviour:  enabled("behaviour"),
		Detentions: enabled("detentions"),
		Timetable:  enabled("timetable"),
		Reports:    enabled("reports"),
		Dinner:     enabled("dinner"),
		Clubs:      enabled("clubs"),
		Trips:      enabled("trips"),
	}, nil
}

// Timetable parses the rendered weekly timetable grid. There is no API
// route for this - MCSTimetable.aspx is server-rendered - so it's scraped
// from the page.
func (c *Client) Timetable() ([]Lesson, error) {
	req, err := c.newRequest(http.MethodGet, c.baseURL+timetablePage, nil)
	if err != nil {
		return nil, &APIError{Message: err.Error()}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &APIError{Message: err.Error()}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &APIError{Message: err.Error()}
	}
	if resp.Request != nil && resp.Request.URL != nil && strings.Contains(resp.Request.URL.String(), "MCSParentLogin") {
		// A lapsed session bounces the page request back to the login form.
		return nil, &AuthError{Message: "Session expired"}
	}
	return parseTimetableHTML(string(body), time.Now()), nil
}

func decodeTableRows(raw string) ([]map[string]any, error) {
	if raw == "" {
		return nil, nil
	}
	var payload struct {
		Table []map[string]any `json:"Table"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, nil
	}
	return payload.Table, nil
}

// Reports fetches school reports published to the parent. The reference
// integration never destructures these beyond a count, so rows are kept as
// raw maps rather than a fixed struct until a real payload sample justifies
// one.
func (c *Client) Reports() ([]map[string]any, error) {
	raw, err := c.get(fmt.Sprintf(epReports, c.session.StudentID))
	if err != nil {
		return nil, err
	}
	return decodeTableRows(raw)
}

// ClubsAndTrips fetches clubs and trips the pupil is enrolled on.
func (c *Client) ClubsAndTrips() ([]map[string]any, error) {
	raw, err := c.get(fmt.Sprintf(epClubs, c.session.StudentID))
	if err != nil {
		return nil, err
	}
	return decodeTableRows(raw)
}

// Detentions fetches detentions recorded for the pupil.
func (c *Client) Detentions() ([]map[string]any, error) {
	raw, err := c.get(fmt.Sprintf(epDetentions, c.session.StudentID))
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	var payload struct {
		Detention []map[string]any `json:"Detention"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, nil
	}
	return payload.Detention, nil
}

// DinnerBalance fetches the dinner money credit balance, scraped from the
// dashboard widget's HTML.
func (c *Client) DinnerBalance() (*DinnerBalance, error) {
	raw, err := c.get(fmt.Sprintf(epDinner, c.session.StudentID))
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	amount, ok := parseDinnerBalance(raw)
	if !ok {
		return nil, nil
	}
	return &DinnerBalance{Amount: amount, Currency: "GBP"}, nil
}

type conversationRow struct {
	RecipientID         int          `json:"RecipientID"`
	RecipientName       string       `json:"RecipientName"`
	UnreadCount         int          `json:"UnreadCount"`
	PublishedDocumentID *int         `json:"PublishedDocumentID"`
	Messages            []messageRow `json:"Messages"`
}

type messageRow struct {
	MessageID   int    `json:"MessageID"`
	Subject     string `json:"Subject"`
	Message     string `json:"Message"`
	MessageDate string `json:"MessageDate"`
	Mode        int    `json:"Mode"` // 0: from the school/teacher, 1: sent by the parent
	IsRead      bool   `json:"IsRead"`
}

type attachmentRow struct {
	AttachmentID int    `json:"AttachmentID"`
	FileName     string `json:"FileName"`
	MessageID    int    `json:"MessageID"`
}

type conversationsPayload struct {
	Conversations      []conversationRow `json:"Conversations"`
	MessageAttachments []attachmentRow   `json:"MessageAttachments"`
}

// Conversations fetches every teacher/school message thread, grouped by
// sender, with attachments joined onto their message and links extracted
// from the plain-text body. MCAS returns the whole inbox in a single call -
// there is no pagination.
func (c *Client) Conversations() ([]Conversation, error) {
	raw, err := c.get(fmt.Sprintf(epConversations, c.session.UserID))
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	var payload conversationsPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, nil
	}

	attachmentsByMessage := map[int][]MessageAttachment{}
	for _, a := range payload.MessageAttachments {
		attachmentsByMessage[a.MessageID] = append(attachmentsByMessage[a.MessageID], MessageAttachment{
			ID:       a.AttachmentID,
			FileName: a.FileName,
		})
	}

	conversations := make([]Conversation, 0, len(payload.Conversations))
	for _, row := range payload.Conversations {
		messages := make([]Message, 0, len(row.Messages))
		for _, m := range row.Messages {
			messages = append(messages, Message{
				ID:          m.MessageID,
				Subject:     m.Subject,
				Body:        m.Message,
				Date:        parseMessageDate(m.MessageDate),
				Sent:        m.Mode == 1,
				Read:        m.IsRead,
				Links:       extractLinks(m.Message),
				Attachments: attachmentsByMessage[m.MessageID],
			})
		}
		conversations = append(conversations, Conversation{
			RecipientID:         row.RecipientID,
			RecipientName:       row.RecipientName,
			UnreadCount:         row.UnreadCount,
			PublishedDocumentID: row.PublishedDocumentID,
			Messages:            messages,
		})
	}
	return conversations, nil
}

// FlattenConversations flattens every thread into a single list of
// messages with their sender inlined, newest first. MCAS doesn't guarantee
// order within or across threads, so ties on Date are broken by higher ID
// first.
func FlattenConversations(conversations []Conversation) []InboxMessage {
	var messages []InboxMessage
	for _, conv := range conversations {
		for _, m := range conv.Messages {
			messages = append(messages, InboxMessage{
				ID:            m.ID,
				RecipientID:   conv.RecipientID,
				RecipientName: conv.RecipientName,
				Subject:       m.Subject,
				Body:          m.Body,
				Date:          m.Date,
				Sent:          m.Sent,
				Read:          m.Read,
				Links:         m.Links,
				Attachments:   m.Attachments,
			})
		}
	}
	sort.SliceStable(messages, func(i, j int) bool {
		if !messages[i].Date.Equal(messages[j].Date) {
			return messages[i].Date.After(messages[j].Date)
		}
		return messages[i].ID > messages[j].ID
	})
	return messages
}

// FindMessage looks up one message by id across every thread.
func FindMessage(conversations []Conversation, id int) (*InboxMessage, error) {
	for _, m := range FlattenConversations(conversations) {
		if m.ID == id {
			return &m, nil
		}
	}
	return nil, &APIError{Message: fmt.Sprintf("no message found with id %d", id)}
}

// MessageAttachmentData downloads and decodes one attachment's raw bytes.
// This call reports no filename or content type - just the base64 body -
// so callers need Conversations()'s MessageAttachment.FileName for that.
//
// The proxy JSON-encodes its response a second time: "d" holds a quoted
// JSON string whose value is the base64, not the bare base64 itself. Bare
// base64 is still accepted as a fallback in case that behaviour differs
// between schools or changes later.
func (c *Client) MessageAttachmentData(messageID, attachmentID int) ([]byte, error) {
	raw, err := c.get(fmt.Sprintf(epMessageAttachment, c.session.UserID, messageID, attachmentID))
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, &APIError{Message: fmt.Sprintf("attachment %d on message %d not found", attachmentID, messageID)}
	}
	payload, err := unquoteProxyString(raw)
	if err != nil {
		return nil, &APIError{Message: "decoding attachment: " + err.Error()}
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload))
	if err != nil {
		return nil, &APIError{Message: "decoding attachment: " + err.Error()}
	}
	return data, nil
}
