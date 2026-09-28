package cmd

import (
	"bytes"
	"os"
	"path/filepath"
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

// chdir switches the working directory to dir for the duration of the test,
// restoring it afterwards. A plain t.TempDir/os.Chdir pair without this
// wouldn't be safe to reuse across subtests since t.Cleanup runs in LIFO
// order regardless of nesting, but each call here is scoped to its own test.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q) error = %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restoring Chdir(%q) error = %v", old, err)
		}
	})
}

func TestResolveAttachmentPathDefaultsToCurrentDirectory(t *testing.T) {
	chdir(t, t.TempDir())

	got, err := resolveAttachmentPath("", "letter.pdf")
	if err != nil {
		t.Fatalf("resolveAttachmentPath() error = %v", err)
	}
	wd, _ := os.Getwd()
	want := filepath.Join(wd, "letter.pdf")
	if got != want {
		t.Errorf("resolveAttachmentPath() = %q, want %q", got, want)
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("resolveAttachmentPath() didn't create %q: %v", got, err)
	}
}

func TestResolveAttachmentPathOutExistingDirectory(t *testing.T) {
	dir := t.TempDir()

	got, err := resolveAttachmentPath(dir, "letter.pdf")
	if err != nil {
		t.Fatalf("resolveAttachmentPath() error = %v", err)
	}
	want := filepath.Join(dir, "letter.pdf")
	if got != want {
		t.Errorf("resolveAttachmentPath() = %q, want %q", got, want)
	}
}

func TestResolveAttachmentPathOutExactFile(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "nested", "note.pdf")
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(out, []byte("old"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := resolveAttachmentPath(out, "ignored-name.pdf")
	if err != nil {
		t.Fatalf("resolveAttachmentPath() error = %v", err)
	}
	if got != out {
		t.Errorf("resolveAttachmentPath() = %q, want %q", got, out)
	}
}

func TestResolveAttachmentPathOutExactFileMissingParentDir(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "new-subdir", "note.pdf")

	got, err := resolveAttachmentPath(out, "ignored-name.pdf")
	if err != nil {
		t.Fatalf("resolveAttachmentPath() error = %v", err)
	}
	if got != out {
		t.Errorf("resolveAttachmentPath() = %q, want %q", got, out)
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("resolveAttachmentPath() didn't create %q: %v", got, err)
	}
}

func TestResolveAttachmentPathDeduplicatesOnCollision(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "letter.pdf"), []byte("first"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "letter (1).pdf"), []byte("second"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := resolveAttachmentPath(dir, "letter.pdf")
	if err != nil {
		t.Fatalf("resolveAttachmentPath() error = %v", err)
	}
	want := filepath.Join(dir, "letter (2).pdf")
	if got != want {
		t.Errorf("resolveAttachmentPath() = %q, want %q", got, want)
	}

	first, err := os.ReadFile(filepath.Join(dir, "letter.pdf"))
	if err != nil || string(first) != "first" {
		t.Errorf("original letter.pdf was modified: content=%q err=%v", first, err)
	}
}

func TestResolveAttachmentPathDeduplicatesNameWithoutExtension(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("first"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := resolveAttachmentPath(dir, "README")
	if err != nil {
		t.Fatalf("resolveAttachmentPath() error = %v", err)
	}
	want := filepath.Join(dir, "README (1)")
	if got != want {
		t.Errorf("resolveAttachmentPath() = %q, want %q", got, want)
	}
}

func TestResolveAttachmentPathSanitizesHostileFileName(t *testing.T) {
	dir := t.TempDir()

	got, err := resolveAttachmentPath(dir, "../../evil.pdf")
	if err != nil {
		t.Fatalf("resolveAttachmentPath() error = %v", err)
	}
	want := filepath.Join(dir, "evil.pdf")
	if got != want {
		t.Errorf("resolveAttachmentPath() = %q, want %q", got, want)
	}

	got, err = resolveAttachmentPath(dir, `a\b.pdf`)
	if err != nil {
		t.Fatalf("resolveAttachmentPath() error = %v", err)
	}
	want = filepath.Join(dir, "b.pdf")
	if got != want {
		t.Errorf("resolveAttachmentPath() = %q, want %q", got, want)
	}
}

func TestPrepareOutDir(t *testing.T) {
	dir := t.TempDir()

	if err := prepareOutDir(dir, 2); err != nil {
		t.Errorf("prepareOutDir(existing dir) error = %v", err)
	}

	created := filepath.Join(dir, "letters")
	if err := prepareOutDir(created, 2); err != nil {
		t.Fatalf("prepareOutDir(missing) error = %v", err)
	}
	if info, err := os.Stat(created); err != nil || !info.IsDir() {
		t.Errorf("prepareOutDir(missing) did not create a directory: %v", err)
	}

	file := filepath.Join(dir, "existing.pdf")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := prepareOutDir(file, 2)
	if err == nil || !strings.Contains(err.Error(), "-o must be a directory when downloading 2 attachments") {
		t.Errorf("prepareOutDir(file) error = %v, want a must-be-a-directory error", err)
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
		"mcas messages 41206 --attachments 52",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q, got:\n%s", want, out)
		}
	}
}
