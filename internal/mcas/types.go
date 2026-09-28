package mcas

import "time"

// Credentials are the only long-lived secret the portal requires - there is
// no OAuth/refresh token anywhere in the auth flow.
type Credentials struct {
	Email    string
	Password string
}

// Session is the pupil context captured off the dashboard after login.
type Session struct {
	StudentID   int
	SchoolID    int
	SchoolName  string
	StudentName string
	YearID      int // lazily populated on first behaviour-related call
	UserID      int // the parent's own user id, needed by the Messages endpoints
}

// ModuleFlags reports which MCAS modules the school has licensed. A module
// the school hasn't enabled reports zero data instead of an error.
type ModuleFlags struct {
	Attendance bool
	Behaviour  bool
	Detentions bool
	Timetable  bool
	Reports    bool
	Dinner     bool
	Clubs      bool
	Trips      bool
}

// AttendancePeriod is one period's registration mark, e.g. Period 1.
type AttendancePeriod struct {
	PeriodName      string `json:"period_name"`
	MarkSign        string `json:"mark_sign"`
	MarkMeaning     string `json:"mark_meaning"`
	MarkDescription string `json:"mark_description"`
	SubjectName     string `json:"subject_name"`
}

// AttendanceDay is one school day's registration marks. MCAS serves a
// single day per call.
type AttendanceDay struct {
	Day     time.Time          `json:"day"`
	Periods []AttendancePeriod `json:"periods"`
}

// Present reports whether every recorded period counts as in school.
// Returns nil when no marks were recorded for the day (weekends, holidays,
// or a day whose register hasn't been taken yet).
func (d AttendanceDay) Present() *bool {
	if len(d.Periods) == 0 {
		return nil
	}
	present := true
	for _, p := range d.Periods {
		if !presentMarkSigns[p.MarkSign] {
			present = false
			break
		}
	}
	return &present
}

// Summary is a human-readable list of the day's marks, one per period.
func (d AttendanceDay) Summary() string {
	if len(d.Periods) == 0 {
		return "No data"
	}
	out := ""
	for i, p := range d.Periods {
		if i > 0 {
			out += ", "
		}
		out += p.PeriodName + ": " + p.MarkMeaning
	}
	return out
}

// DayType categorises a calendar day per the school's own academic
// calendar, decoded from the behaviour year payload's DayStatusCode.
type DayType string

const (
	SchoolDay DayType = "school_day"
	Weekend   DayType = "weekend"
	Holiday   DayType = "holiday"
	StaffDay  DayType = "staff_day"
	Unknown   DayType = "unknown"
)

// BehaviourEvent is one recorded behaviour event (a merit, a sanction, etc).
type BehaviourEvent struct {
	ID      int       `json:"id"`
	Date    time.Time `json:"date"`
	Type    string    `json:"type"`   // "Positive" / "Negative" / "Neutral"
	Points  int       `json:"points"` // signed: the raw MCAS adjustment
	Subject string    `json:"subject"`

	// Filled from the per-day view (see ApplyDayDetails); empty when that
	// view couldn't be loaded or didn't line up with the year data.
	Description string `json:"description,omitempty"`
	Teacher     string `json:"teacher,omitempty"`
	Class       string `json:"class,omitempty"`
	Outcome     string `json:"outcome,omitempty"`
}

// BehaviourDayEvent is one row of the per-day behaviour table. The rows carry
// no ID or time; they come back in the same order as the year call's events.
type BehaviourDayEvent struct {
	Date        string `json:"date"` // ISO YYYY-MM-DD
	Class       string `json:"class"`
	Teacher     string `json:"teacher"`
	Description string `json:"description"`
	Outcome     string `json:"outcome"`
	Type        string `json:"type"` // "Positive" / "Negative", or "" when the row has no known icon
}

// BehaviourPoints are the point totals for the academic year. The negative
// totals (Negative, AllTimeNegative) are absolute values. The AllTime*
// fields are nil when the school hides that figure (MCAS reports "N/A").
type BehaviourPoints struct {
	Total           int  `json:"total"`
	Positive        int  `json:"positive"`
	Negative        int  `json:"negative"`
	AllTimeTotal    *int `json:"all_time_total,omitempty"`
	AllTimePositive *int `json:"all_time_positive,omitempty"`
	AllTimeNegative *int `json:"all_time_negative,omitempty"`
}

// BehaviourYear is the whole academic year's behaviour data in one call:
// richer and cheaper than walking days, and the only source of the point
// totals.
type BehaviourYear struct {
	YearName string             `json:"year_name"`
	Events   []BehaviourEvent   `json:"events"`
	Points   BehaviourPoints    `json:"points"`
}

// CalendarDay is one day of the school's academic calendar.
type CalendarDay struct {
	Date string  `json:"date"` // ISO YYYY-MM-DD
	Type DayType `json:"type"`
}

// AcademicCalendar is the school's calendar for the academic year, sorted
// by date.
type AcademicCalendar struct {
	YearName string        `json:"year_name"`
	Days     []CalendarDay `json:"days"`
}

// Lesson is one timetabled lesson. Date is nil when the year-guessing
// heuristic (see ParseDayHeader) couldn't resolve a year for the header.
type Lesson struct {
	Day     string     `json:"day"`
	Date    *time.Time `json:"date,omitempty"`
	Period  string     `json:"period"`
	Subject string     `json:"subject"`
	Class   string     `json:"class"`
	Teacher string     `json:"teacher"`
}

// DinnerBalance is the dinner money credit balance, scraped from the
// dashboard widget's HTML.
type DinnerBalance struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"` // always "GBP" - MCAS only serves UK schools
}

// Conversation is a message thread with one sender (a teacher or the school).
type Conversation struct {
	RecipientID         int       `json:"recipient_id"`
	RecipientName       string    `json:"recipient_name,omitempty"` // can be empty for the school itself
	UnreadCount         int       `json:"unread_count"`
	PublishedDocumentID *int      `json:"published_document_id,omitempty"` // set for report-comment threads
	Messages            []Message `json:"messages"`
}

// Message is one message within a Conversation. Bodies are plain text, not
// HTML - Links is extracted from bare URLs found in Body.
type Message struct {
	ID          int                 `json:"id"`
	Subject     string              `json:"subject"`
	Body        string              `json:"body"`
	Date        time.Time           `json:"date"`
	Sent        bool                `json:"sent"` // false: from the school/teacher, true: sent by the parent
	Read        bool                `json:"read"`
	Links       []string            `json:"links,omitempty"`
	Attachments []MessageAttachment `json:"attachments,omitempty"`
}

// MessageAttachment is a file attached to a Message. MCAS reports only a
// file name here - no MIME type or size - so callers infer type from the
// name's extension.
type MessageAttachment struct {
	ID       int    `json:"id"`
	FileName string `json:"file_name"`
}

// InboxMessage is one message with its sender inlined, for flat listing.
type InboxMessage struct {
	ID            int                 `json:"id"`
	RecipientID   int                 `json:"recipient_id"`
	RecipientName string              `json:"recipient_name,omitempty"` // can be empty for the school itself
	Subject       string              `json:"subject"`
	Body          string              `json:"body"`
	Date          time.Time           `json:"date"`
	Sent          bool                `json:"sent"` // false: from the school/teacher, true: sent by the parent
	Read          bool                `json:"read"`
	Links         []string            `json:"links,omitempty"`
	Attachments   []MessageAttachment `json:"attachments,omitempty"`
}
