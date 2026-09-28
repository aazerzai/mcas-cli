package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/dental-dash/my-child-at-school-cli/internal/mcas"
)

func TestSanitizeFileNameStripsPathSeparators(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"letter.pdf", "letter.pdf"},
		{"../../etc/passwd", "passwd"},
		{"sub/dir\\name.pdf", "name.pdf"},
		{"", "attachment"},
	}
	for _, tt := range tests {
		if got := sanitizeFileName(tt.in); got != tt.want {
			t.Errorf("sanitizeFileName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFindAttachmentFileName(t *testing.T) {
	conversations := []mcas.Conversation{
		{
			RecipientID: 508,
			Messages: []mcas.Message{
				{
					ID: 1001,
					Attachments: []mcas.MessageAttachment{
						{ID: 52, FileName: "letter.pdf"},
					},
				},
			},
		},
	}
	if got := findAttachmentFileName(conversations, 1001, 52); got != "letter.pdf" {
		t.Errorf("findAttachmentFileName() = %q, want letter.pdf", got)
	}
	if got := findAttachmentFileName(conversations, 1001, 999); got != "" {
		t.Errorf("findAttachmentFileName() = %q, want empty for an unknown attachment id", got)
	}
}

// resetMessagesFlags returns messagesCmd's list flags to their unset,
// default state, both immediately and again at the end of the test - Set()
// marks a flag Changed even when writing back its default value, so a plain
// value reset isn't enough to undo a Changed()-based check.
func resetMessagesFlags(t *testing.T) {
	t.Helper()
	reset := func() {
		for _, name := range []string{"limit", "from", "unread", "since"} {
			f := messagesCmd.Flags().Lookup(name)
			if err := f.Value.Set(f.DefValue); err != nil {
				t.Fatalf("resetting --%s: %v", name, err)
			}
			f.Changed = false
		}
	}
	reset()
	t.Cleanup(reset)
}

func TestRunMessageByIDRejectsListFlags(t *testing.T) {
	tests := []struct {
		flag  string
		value string
	}{
		{"limit", "5"},
		{"from", "508"},
		{"unread", "true"},
		{"since", "2026-01-01"},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			resetMessagesFlags(t)
			if err := messagesCmd.Flags().Set(tt.flag, tt.value); err != nil {
				t.Fatalf("Flags().Set(%q) error = %v", tt.flag, err)
			}
			err := runMessageByID(messagesCmd, "41206")
			want := "--" + tt.flag + " can't be used with a message id"
			if err == nil || err.Error() != want {
				t.Errorf("runMessageByID() error = %v, want %q", err, want)
			}
		})
	}
}

func TestRunMessageByIDRejectsNonIntegerID(t *testing.T) {
	resetMessagesFlags(t)
	err := runMessageByID(messagesCmd, "x")
	want := `invalid message id "x": want an integer`
	if err == nil || err.Error() != want {
		t.Errorf("runMessageByID() error = %v, want %q", err, want)
	}
}

func TestParseSinceFlagEmptyMeansNoBound(t *testing.T) {
	got, err := parseSinceFlag("")
	if err != nil {
		t.Fatalf(`parseSinceFlag("") error = %v`, err)
	}
	if got != nil {
		t.Errorf(`parseSinceFlag("") = %v, want nil`, got)
	}
}

func TestParseSinceFlagRejectsInvalidDate(t *testing.T) {
	_, err := parseSinceFlag("2026-13-01")
	want := `invalid --since "2026-13-01": want YYYY-MM-DD`
	if err == nil || err.Error() != want {
		t.Errorf("parseSinceFlag() error = %v, want %q", err, want)
	}
}

func TestFilterMessagesCombinesFromUnreadSinceAndLimit(t *testing.T) {
	t.Cleanup(func() { messagesFrom, messagesUnread, messagesLimit = 0, false, 0 })

	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	messages := []mcas.InboxMessage{
		{ID: 1, RecipientID: 508, Read: true, Date: base.AddDate(0, 0, 10)},  // read: excluded by --unread
		{ID: 2, RecipientID: 508, Read: false, Date: base.AddDate(0, 0, 5)},  // matches every filter
		{ID: 3, RecipientID: 462, Read: false, Date: base.AddDate(0, 0, 20)}, // wrong recipient: excluded by --from
		{ID: 4, RecipientID: 508, Read: false, Date: base.AddDate(0, 0, -5)}, // too old: excluded by --since
	}

	messagesFrom = 508
	messagesUnread = true
	messagesLimit = 5
	since := base

	got := filterMessages(messages, true, &since)
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("filterMessages() = %+v, want just message id=2", got)
	}
}

func TestFilterMessagesLimitAppliesAfterFiltering(t *testing.T) {
	t.Cleanup(func() { messagesFrom, messagesUnread, messagesLimit = 0, false, 0 })

	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	messages := []mcas.InboxMessage{
		{ID: 1, RecipientID: 508, Date: base.AddDate(0, 0, 3)},
		{ID: 2, RecipientID: 508, Date: base.AddDate(0, 0, 2)},
		{ID: 3, RecipientID: 999, Date: base.AddDate(0, 0, 1)}, // filtered out by --from before --limit counts
	}

	messagesFrom = 508
	messagesLimit = 1

	got := filterMessages(messages, true, nil)
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("filterMessages() = %+v, want just message id=1", got)
	}
}

func TestRenderInboxMessagesTextListsFields(t *testing.T) {
	messages := []mcas.InboxMessage{
		{
			ID:            52311,
			RecipientName: "Mrs S Patel",
			Subject:       "Weekly Newsletter",
			Date:          time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC),
			Read:          true,
			Attachments:   []mcas.MessageAttachment{{ID: 1, FileName: "a.pdf"}},
		},
		{
			ID:      51702,
			Subject: "Term dates reminder",
			Date:    time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
			Read:    false,
		},
	}
	var buf bytes.Buffer
	if err := renderInboxMessagesText(&buf, messages); err != nil {
		t.Fatalf("renderInboxMessagesText() error = %v", err)
	}
	out := buf.String()
	for _, want := range []string{"52311", "2026-09-24", "Mrs S Patel", "[1 att]", "(unknown sender)", "[unread]"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
}

func TestRenderInboxMessagesTextEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := renderInboxMessagesText(&buf, []mcas.InboxMessage{}); err != nil {
		t.Fatalf("renderInboxMessagesText() error = %v", err)
	}
	if buf.String() != "No messages.\n" {
		t.Errorf("output = %q, want %q", buf.String(), "No messages.\n")
	}
}

func TestRenderInboxMessageTextIncludesAttachmentCommand(t *testing.T) {
	m := &mcas.InboxMessage{
		ID:            41206,
		RecipientID:   508,
		RecipientName: "Mrs S Patel",
		Subject:       "Weekly Newsletter - 01.05.25",
		Body:          "Dear Parent, Carer of ...",
		Date:          time.Date(2025, 5, 1, 14, 7, 0, 0, time.UTC),
		Read:          true,
		Links:         []string{"https://sway.cloud.microsoft/abc"},
		Attachments:   []mcas.MessageAttachment{{ID: 52, FileName: "newsletter.pdf"}},
	}
	var buf bytes.Buffer
	if err := renderInboxMessageText(&buf, m); err != nil {
		t.Fatalf("renderInboxMessageText() error = %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"From:      Mrs S Patel (recipient id 508)",
		"Date:      2025-05-01 14:07",
		"Direction: Received",
		"Read:      yes",
		"Subject:   Weekly Newsletter - 01.05.25",
		"Dear Parent, Carer of ...",
		"https://sway.cloud.microsoft/abc",
		"mcas messages attachment 41206 52",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
}
