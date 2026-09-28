package mcas

import "testing"

func TestNormalizeFileName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Year 10 Induction Evening    .pdf", "Year 10 Induction Evening.pdf"},
		{"18  Parents Carers and Families Newsletter 8.7.26_compressed.pdf", "18 Parents Carers and Families Newsletter 8.7.26_compressed.pdf"},
		{"Farewell Message for Miss Tai .pdf", "Farewell Message for Miss Tai.pdf"},
		{"  Sports Day – Hot Weather Conditions .pdf ", "Sports Day – Hot Weather Conditions.pdf"},
		{"Notes\t v2 .docx", "Notes v2.docx"},
		{"README", "README"},
		{"   ", "   "},
		{" .pdf", ".pdf"},
	}
	for _, tt := range tests {
		if got := normalizeFileName(tt.in); got != tt.want {
			t.Errorf("normalizeFileName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
