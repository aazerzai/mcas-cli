// Package config resolves MCAS credentials and CLI preferences.
//
// The only long-lived secret the auth flow requires is the email+password
// pair itself - there's no OAuth/refresh token to manage. Passwords are
// kept out of the config file entirely: they live in the OS keychain, with
// environment variables and flags available as override/CI escape hatches.
// Resolution order: --email/--password flags, then MCAS_EMAIL/MCAS_PASSWORD
// env vars, then the OS keychain entry written by `mcas login`.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
	"github.com/zalando/go-keyring"

	"github.com/dental-dash/my-child-at-school-cli/internal/mcas"
)

const (
	keyringService  = "my-child-at-school-cli"
	defaultCacheTTL = 15 * time.Minute
	appDirName      = "my-child-at-school-cli"
)

// ErrNoCredentials is returned when no credentials could be resolved from
// flags, environment variables, or the keychain.
var ErrNoCredentials = errors.New("no MCAS credentials found - run 'mcas login', set MCAS_EMAIL/MCAS_PASSWORD, or pass --email/--password")

// Config wraps the CLI's non-secret preferences (default output format,
// the last signed-in email for display, cache TTL).
type Config struct {
	v    *viper.Viper
	path string
}

// Load reads the config file (creating its directory if needed) and wires
// up MCAS_-prefixed environment variable overrides.
func Load() (*Config, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "config.yaml")

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetEnvPrefix("MCAS")
	v.AutomaticEnv()
	v.SetDefault("output", "text")
	v.SetDefault("cache_ttl_minutes", int(defaultCacheTTL.Minutes()))

	if err := v.ReadInConfig(); err != nil {
		if !errors.As(err, &viper.ConfigFileNotFoundError{}) && !os.IsNotExist(err) {
			return nil, err
		}
	}
	return &Config{v: v, path: path}, nil
}

func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appDirName), nil
}

func cacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appDirName), nil
}

// CacheDir returns the directory the on-disk cache should use.
func (c *Config) CacheDir() (string, error) {
	return cacheDir()
}

// CacheTTL returns the configured cache lifetime.
func (c *Config) CacheTTL() time.Duration {
	return time.Duration(c.v.GetInt("cache_ttl_minutes")) * time.Minute
}

// DefaultOutputFormat returns the configured default --output value.
func (c *Config) DefaultOutputFormat() string {
	return c.v.GetString("output")
}

// StoredEmail returns the email address saved by the last successful
// `mcas login`, if any.
func (c *Config) StoredEmail() string {
	return c.v.GetString("email")
}

// SaveCredentials stores the password in the OS keychain and the email
// address (non-secret) in the config file, so subsequent commands know
// which keychain entry to read.
func (c *Config) SaveCredentials(email, password string) error {
	if err := keyring.Set(keyringService, email, password); err != nil {
		return err
	}
	c.v.Set("email", email)
	return c.v.WriteConfigAs(c.path)
}

// ClearCredentials removes the stored password from the OS keychain and
// forgets the associated email address.
func (c *Config) ClearCredentials() error {
	email := c.StoredEmail()
	if email == "" {
		return nil
	}
	if err := keyring.Delete(keyringService, email); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	c.v.Set("email", "")
	return c.v.WriteConfigAs(c.path)
}

// ResolveCredentials applies the resolution order documented on this
// package: explicit flags, then environment variables, then the keychain
// entry written by `mcas login`.
func (c *Config) ResolveCredentials(flagEmail, flagPassword string) (mcas.Credentials, error) {
	if flagEmail != "" && flagPassword != "" {
		return mcas.Credentials{Email: flagEmail, Password: flagPassword}, nil
	}
	if email, password := os.Getenv("MCAS_EMAIL"), os.Getenv("MCAS_PASSWORD"); email != "" && password != "" {
		return mcas.Credentials{Email: email, Password: password}, nil
	}
	email := c.StoredEmail()
	if email == "" {
		return mcas.Credentials{}, ErrNoCredentials
	}
	password, err := keyring.Get(keyringService, email)
	if err != nil {
		return mcas.Credentials{}, ErrNoCredentials
	}
	return mcas.Credentials{Email: email, Password: password}, nil
}
