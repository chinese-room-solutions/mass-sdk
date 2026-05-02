// Package format provides small, allocation-free string formatters for the
// MASS ecosystem. Pure Go, no UI/HTML dependencies — safe to import from any
// backend, gateway, worker, or CLI without dragging in templ/CSS/JS.
package format

import (
	"fmt"
	"strings"
)

// Bytes formats a byte count as a human-readable size string with binary
// units (1 KiB = 1024 B). Examples: "0 B", "1.0 KB", "4.2 GB".
func Bytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// Count formats a number with K/M suffixes, base-10 (1k = 1000). Examples:
// "42", "1.5k", "2.5M".
func Count(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// Params formats a parameter count as the convention used by model cards
// ("7B", "1.5B", "400M"). Base 10.
func Params(n int64) string {
	switch {
	case n >= 1_000_000_000:
		v := float64(n) / 1_000_000_000
		if v == float64(int64(v)) {
			return fmt.Sprintf("%dB", int64(v))
		}
		return fmt.Sprintf("%.1fB", v)
	case n >= 1_000_000:
		v := float64(n) / 1_000_000
		if v == float64(int64(v)) {
			return fmt.Sprintf("%dM", int64(v))
		}
		return fmt.Sprintf("%.0fM", v)
	default:
		return fmt.Sprintf("%dK", n/1_000)
	}
}

// Truncate shortens s to maxLen, replacing the dropped tail with "..." when
// truncation occurs. Returns s unchanged when len(s) <= maxLen.
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// JSONEscape escapes a string for safe embedding inside a JSON string value
// without going through encoding/json. Cheaper than json.Marshal for hot
// paths that just need backslash + quote + newline handling.
func JSONEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return r.Replace(s)
}
