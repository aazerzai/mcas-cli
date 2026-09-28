package cmd

import (
	"fmt"
	"time"
)

const dateFormat = "2006-01-02"

// parseDateFlag parses a --date flag value, defaulting to today when empty.
func parseDateFlag(value string) (time.Time, error) {
	if value == "" {
		return time.Now(), nil
	}
	t, err := time.Parse(dateFormat, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q: want YYYY-MM-DD", value)
	}
	return t, nil
}
