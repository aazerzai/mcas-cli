package cmd

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// renderRawRows renders a []map[string]any as returned by endpoints whose
// schema isn't modeled yet (detentions, reports, clubs and trips) - see the
// analysis in issue #3 on why these stay untyped for now.
func renderRawRows(w io.Writer, data any, noneMessage string) error {
	rows := data.([]map[string]any)
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, noneMessage)
		return err
	}
	for _, row := range rows {
		keys := make([]string, 0, len(row))
		for k := range row {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s: %v", k, row[k]))
		}
		if _, err := fmt.Fprintln(w, "-", strings.Join(parts, ", ")); err != nil {
			return err
		}
	}
	return nil
}
