// Package output renders command results as either human-readable text or
// a stable JSON envelope intended for AI tools/agents to consume without
// per-command schema knowledge.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

const schemaVersion = "1"

// Format selects how a command's result is rendered.
type Format string

const (
	Text Format = "text"
	JSON Format = "json"
)

// ErrorType categorises a failure so a script/agent can branch on it
// without string-matching messages.
type ErrorType string

const (
	AuthErrorType ErrorType = "auth_error"
	APIErrorType  ErrorType = "api_error"
)

type envelopeError struct {
	Type    ErrorType `json:"type"`
	Message string    `json:"message"`
}

type envelope struct {
	SchemaVersion string         `json:"schema_version"`
	Command       string         `json:"command"`
	GeneratedAt   time.Time      `json:"generated_at"`
	Data          any            `json:"data"`
	Error         *envelopeError `json:"error"`
}

// TextRenderer produces the human-readable form of a command's data. Each
// command supplies its own, since the shape of "data" varies per command.
type TextRenderer func(w io.Writer, data any) error

// Result writes a successful command result in the requested format.
func Result(w io.Writer, format Format, command string, data any, render TextRenderer) error {
	if format == JSON {
		return writeEnvelope(w, envelope{
			SchemaVersion: schemaVersion,
			Command:       command,
			GeneratedAt:   time.Now().UTC(),
			Data:          data,
		})
	}
	return render(w, data)
}

// Failure writes a command failure in the requested format. Exit code
// handling is the caller's responsibility - this only renders.
func Failure(w io.Writer, format Format, command string, errType ErrorType, err error) error {
	if format == JSON {
		return writeEnvelope(w, envelope{
			SchemaVersion: schemaVersion,
			Command:       command,
			GeneratedAt:   time.Now().UTC(),
			Error:         &envelopeError{Type: errType, Message: err.Error()},
		})
	}
	_, writeErr := fmt.Fprintln(w, "Error:", err.Error())
	return writeErr
}

func writeEnvelope(w io.Writer, env envelope) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(env)
}
