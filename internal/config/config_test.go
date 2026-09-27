package config

import "testing"

func newTestConfig(t *testing.T) *Config {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return cfg
}

func TestResolveCredentialsPrefersFlags(t *testing.T) {
	cfg := newTestConfig(t)
	creds, err := cfg.ResolveCredentials("flag@example.com", "flag-password")
	if err != nil {
		t.Fatalf("ResolveCredentials() error = %v", err)
	}
	if creds.Email != "flag@example.com" || creds.Password != "flag-password" {
		t.Errorf("creds = %+v", creds)
	}
}

func TestResolveCredentialsFallsBackToEnv(t *testing.T) {
	cfg := newTestConfig(t)
	t.Setenv("MCAS_EMAIL", "env@example.com")
	t.Setenv("MCAS_PASSWORD", "env-password")
	creds, err := cfg.ResolveCredentials("", "")
	if err != nil {
		t.Fatalf("ResolveCredentials() error = %v", err)
	}
	if creds.Email != "env@example.com" || creds.Password != "env-password" {
		t.Errorf("creds = %+v", creds)
	}
}

func TestResolveCredentialsErrorsWhenNoneAvailable(t *testing.T) {
	cfg := newTestConfig(t)
	if _, err := cfg.ResolveCredentials("", ""); err != ErrNoCredentials {
		t.Errorf("ResolveCredentials() error = %v, want ErrNoCredentials", err)
	}
}

func TestDefaultsAreSane(t *testing.T) {
	cfg := newTestConfig(t)
	if got := cfg.DefaultOutputFormat(); got != "text" {
		t.Errorf("DefaultOutputFormat() = %q, want text", got)
	}
	if got := cfg.CacheTTL(); got.Minutes() != 15 {
		t.Errorf("CacheTTL() = %v, want 15m", got)
	}
}
