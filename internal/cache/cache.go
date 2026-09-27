// Package cache is a short-TTL on-disk cache for MCAS responses.
//
// An AI agent driving this CLI has no natural pacing the way a human
// clicking through the portal, or the reference Home Assistant
// integration's 30-minute poll loop, does. Serving repeated invocations
// from a local cache within a short window avoids hammering MCAS on every
// call, while --force-refresh remains available as an explicit escape
// hatch.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Cache reads and writes JSON-encoded entries under dir, each valid for ttl
// after it was written.
type Cache struct {
	dir string
	ttl time.Duration
}

// New creates a Cache rooted at dir, creating it if necessary.
func New(dir string, ttl time.Duration) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Cache{dir: dir, ttl: ttl}, nil
}

type entry struct {
	CachedAt time.Time       `json:"cached_at"`
	Value    json.RawMessage `json:"value"`
}

func (c *Cache) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:])+".json")
}

// Get reports whether a fresh (within ttl) entry exists for key and, if so,
// decodes it into out.
func (c *Cache) Get(key string, out any) (bool, error) {
	data, err := os.ReadFile(c.path(key))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var e entry
	if err := json.Unmarshal(data, &e); err != nil {
		return false, nil // treat a corrupt cache entry as a miss
	}
	if time.Since(e.CachedAt) > c.ttl {
		return false, nil
	}
	if err := json.Unmarshal(e.Value, out); err != nil {
		return false, nil
	}
	return true, nil
}

// Set writes value for key, timestamped now.
func (c *Cache) Set(key string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data, err := json.Marshal(entry{CachedAt: time.Now(), Value: encoded})
	if err != nil {
		return err
	}
	return os.WriteFile(c.path(key), data, 0o600)
}
