package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/dental-dash/my-child-at-school-cli/internal/cache"
	"github.com/dental-dash/my-child-at-school-cli/internal/config"
	"github.com/dental-dash/my-child-at-school-cli/internal/mcas"
	"github.com/dental-dash/my-child-at-school-cli/internal/output"
)

// exitError signals that a command already rendered its failure to stdout
// in the requested output format; Execute() just needs a non-nil error to
// exit non-zero without Cobra printing anything further.
type exitError struct{}

func (exitError) Error() string { return "" }

func resolveFormat(cfg *config.Config) (output.Format, error) {
	value := outputFormat
	if value == "" {
		value = cfg.DefaultOutputFormat()
	}
	switch value {
	case "", string(output.Text):
		return output.Text, nil
	case string(output.JSON):
		return output.JSON, nil
	default:
		return "", fmt.Errorf("invalid --output %q: must be %q or %q", value, output.Text, output.JSON)
	}
}

// fetchCached serves fetch's result from the on-disk cache when a fresh
// entry exists for key, unless force bypasses it.
func fetchCached[T any](c *cache.Cache, key string, force bool, fetch func() (T, error)) (T, error) {
	var out T
	if !force {
		if hit, err := c.Get(key, &out); err == nil && hit {
			return out, nil
		}
	}
	value, err := fetch()
	if err != nil {
		return out, err
	}
	_ = c.Set(key, value)
	return value, nil
}

// runCommand is the shared body of every data-fetching command: resolve
// output format and credentials, serve from cache when possible, log in
// and call fetch on a cache miss, then render the result (or failure) in
// the requested format.
func runCommand[T any](name, cacheKeyExtra string, fetch func(client *mcas.Client) (T, error), render output.TextRenderer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	format, err := resolveFormat(cfg)
	if err != nil {
		return err
	}

	creds, err := cfg.ResolveCredentials(flagEmail, flagPassword)
	if err != nil {
		return emitFailure(format, name, output.AuthErrorType, err)
	}

	cacheDir, err := cfg.CacheDir()
	if err != nil {
		return err
	}
	c, err := cache.New(cacheDir, cfg.CacheTTL())
	if err != nil {
		return err
	}

	key := name + ":" + creds.Email + ":" + cacheKeyExtra
	data, err := fetchCached(c, key, forceRefresh, func() (T, error) {
		client := mcas.New(creds)
		if err := client.Login(); err != nil {
			var zero T
			return zero, err
		}
		return fetch(client)
	})
	if err != nil {
		errType := output.APIErrorType
		var authErr *mcas.AuthError
		if errors.As(err, &authErr) {
			errType = output.AuthErrorType
		}
		return emitFailure(format, name, errType, err)
	}

	if err := output.Result(os.Stdout, format, name, data, render); err != nil {
		return err
	}
	return nil
}

func emitFailure(format output.Format, name string, errType output.ErrorType, err error) error {
	_ = output.Failure(os.Stdout, format, name, errType, err)
	return exitError{}
}
