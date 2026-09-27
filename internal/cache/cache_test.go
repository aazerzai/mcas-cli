package cache

import (
	"testing"
	"time"
)

func TestGetSetRoundTrip(t *testing.T) {
	c, err := New(t.TempDir(), time.Hour)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	type payload struct{ Name string }
	if err := c.Set("key", payload{Name: "Alex"}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	var got payload
	hit, err := c.Get("key", &got)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !hit {
		t.Fatal("Get() reported a miss right after Set()")
	}
	if got.Name != "Alex" {
		t.Errorf("got.Name = %q, want Alex", got.Name)
	}
}

func TestGetMissesWhenEntryExpired(t *testing.T) {
	c, err := New(t.TempDir(), -time.Second) // already expired
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.Set("key", "value"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	var got string
	hit, err := c.Get("key", &got)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if hit {
		t.Error("Get() reported a hit for an expired entry")
	}
}

func TestGetMissesWhenAbsent(t *testing.T) {
	c, err := New(t.TempDir(), time.Hour)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var got string
	hit, err := c.Get("missing", &got)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if hit {
		t.Error("Get() reported a hit for a key that was never set")
	}
}
