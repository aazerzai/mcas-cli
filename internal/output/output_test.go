package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestResultJSONEnvelope(t *testing.T) {
	var buf bytes.Buffer
	err := Result(&buf, JSON, "attendance", map[string]string{"day": "present"}, nil)
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if decoded["schema_version"] != schemaVersion {
		t.Errorf("schema_version = %v, want %v", decoded["schema_version"], schemaVersion)
	}
	if decoded["command"] != "attendance" {
		t.Errorf("command = %v, want attendance", decoded["command"])
	}
	if decoded["error"] != nil {
		t.Errorf("error = %v, want nil", decoded["error"])
	}
}

func TestResultTextUsesRenderer(t *testing.T) {
	var buf bytes.Buffer
	called := false
	err := Result(&buf, Text, "attendance", "data", func(w io.Writer, data any) error {
		called = true
		_, err := w.Write([]byte("rendered: " + data.(string)))
		return err
	})
	if err != nil {
		t.Fatalf("Result() error = %v", err)
	}
	if !called {
		t.Error("text renderer was not invoked")
	}
	if buf.String() != "rendered: data" {
		t.Errorf("buf = %q", buf.String())
	}
}

func TestFailureJSONEnvelope(t *testing.T) {
	var buf bytes.Buffer
	err := Failure(&buf, JSON, "attendance", AuthErrorType, errors.New("bad credentials"))
	if err != nil {
		t.Fatalf("Failure() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if decoded["data"] != nil {
		t.Errorf("data = %v, want nil", decoded["data"])
	}
	errObj, ok := decoded["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %v, want an object", decoded["error"])
	}
	if errObj["type"] != string(AuthErrorType) {
		t.Errorf("error.type = %v, want %v", errObj["type"], AuthErrorType)
	}
	if errObj["message"] != "bad credentials" {
		t.Errorf("error.message = %v, want 'bad credentials'", errObj["message"])
	}
}

func TestFailureTextPrintsMessage(t *testing.T) {
	var buf bytes.Buffer
	if err := Failure(&buf, Text, "attendance", APIErrorType, errors.New("boom")); err != nil {
		t.Fatalf("Failure() error = %v", err)
	}
	if !strings.Contains(buf.String(), "boom") {
		t.Errorf("buf = %q, want it to contain 'boom'", buf.String())
	}
}
