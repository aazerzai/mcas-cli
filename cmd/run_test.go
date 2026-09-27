package cmd

import (
	"testing"

	"github.com/dental-dash/my-child-at-school-cli/internal/config"
	"github.com/dental-dash/my-child-at-school-cli/internal/output"
)

func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	return cfg
}

func TestResolveFormatDefaultsToText(t *testing.T) {
	cfg := newTestConfig(t)
	outputFormat = ""
	format, err := resolveFormat(cfg)
	if err != nil {
		t.Fatalf("resolveFormat() error = %v", err)
	}
	if format != output.Text {
		t.Errorf("format = %q, want %q", format, output.Text)
	}
}

func TestResolveFormatHonoursFlag(t *testing.T) {
	cfg := newTestConfig(t)
	t.Cleanup(func() { outputFormat = "" })
	outputFormat = "json"
	format, err := resolveFormat(cfg)
	if err != nil {
		t.Fatalf("resolveFormat() error = %v", err)
	}
	if format != output.JSON {
		t.Errorf("format = %q, want %q", format, output.JSON)
	}
}

func TestResolveFormatRejectsUnknownValue(t *testing.T) {
	cfg := newTestConfig(t)
	t.Cleanup(func() { outputFormat = "" })
	outputFormat = "xml"
	if _, err := resolveFormat(cfg); err == nil {
		t.Error("resolveFormat() error = nil, want an error for an invalid format")
	}
}
