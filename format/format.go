// Package format provides small, allocation-free string formatters for the
// MASS ecosystem. Pure Go, no UI/HTML dependencies — safe to import from any
// backend, gateway, worker, or CLI without dragging in templ/CSS/JS.
package format

import (
	"fmt"
	"strings"
)

// Bytes formats a byte count as a human-readable size string with binary
// units (1 KiB = 1024 B), labelled as such. Examples: "0 B", "1.0 KiB",
// "4.2 GiB".
func Bytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(b)/float64(1<<10))
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

// Truncate shortens s to at most maxLen runes, replacing the dropped tail with
// "..." when truncation occurs. Rune-safe: never splits a multi-byte character.
// When maxLen < 3 there is no room for the ellipsis, so a longer s is cut to
// its first maxLen runes (maxLen <= 0 yields "").
func Truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	if maxLen < 3 {
		return string(r[:maxLen])
	}
	return string(r[:maxLen-3]) + "..."
}

// JSONEscape escapes a string for safe embedding inside a JSON string value
// without going through encoding/json. Cheaper than json.Marshal for hot
// paths that just need backslash + quote + newline handling.
func JSONEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return r.Replace(s)
}

// JSEscape escapes a string for safe embedding inside a JavaScript string
// literal — escapes both single and double quotes plus newlines, and "<" (as
// \u003c) so a value containing "</script>" can't break out of a surrounding
// <script> body. Use for inline event-handler attributes (e.g.
// `data-on:click="foo('{value}')"`) where the surrounding quote style may vary.
func JSEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `'`, `\'`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "<", `\u003c`)
	return r.Replace(s)
}
