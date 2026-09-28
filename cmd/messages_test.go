package cmd

import (
	"testing"

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
