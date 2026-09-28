package cmd

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestPromptLineAndPromptPasswordShareReader(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("email\npassword\n"))
	var out bytes.Buffer

	email, err := promptLine(reader, &out, "Email: ")
	if err != nil {
		t.Fatalf("promptLine() error = %v", err)
	}
	if email != "email" {
		t.Errorf("email = %q, want %q", email, "email")
	}

	password, err := promptPassword(reader, &out, "Password: ")
	if err != nil {
		t.Fatalf("promptPassword() error = %v", err)
	}
	if password != "password" {
		t.Errorf("password = %q, want %q", password, "password")
	}
}
