package format

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestBytes(t *testing.T) {
	tests := []struct {
		name string
		b    int64
		want string
	}{
		{"zero bytes", 0, "0 B"},
		{"one byte", 1, "1 B"},
		{"below KiB", 1023, "1023 B"},
		{"exactly 1 KiB", 1024, "1.0 KiB"},
		{"1.5 KiB", 1536, "1.5 KiB"},
		{"below MiB", 1<<20 - 1, "1024.0 KiB"},
		{"exactly 1 MiB", 1 << 20, "1.0 MiB"},
		{"500 MiB", 500 << 20, "500.0 MiB"},
		{"below GiB", 1<<30 - 1, "1024.0 MiB"},
		{"exactly 1 GiB", 1 << 30, "1.0 GiB"},
		{"4.2 GiB", 4509715660, "4.2 GiB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, Bytes(tt.b))
		})
	}
}

func TestCount(t *testing.T) {
	tests := []struct {
		name string
		n    int64
		want string
	}{
		{"zero", 0, "0"},
		{"small number", 42, "42"},
		{"999", 999, "999"},
		{"exactly 1k", 1000, "1.0k"},
		{"1.5k", 1500, "1.5k"},
		{"999k", 999_999, "1000.0k"},
		{"exactly 1M", 1_000_000, "1.0M"},
		{"2.5M", 2_500_000, "2.5M"},
		{"large number", 100_000_000, "100.0M"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, Count(tt.n))
		})
	}
}

func TestParams(t *testing.T) {
	tests := []struct {
		name string
		n    int64
		want string
	}{
		{"7B", 7_000_000_000, "7B"},
		{"1.5B", 1_500_000_000, "1.5B"},
		{"400M", 400_000_000, "400M"},
		{"sub-million falls to K", 500, "0K"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, Params(tt.n))
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"short stays", "hello", 10, "hello"},
		{"exact stays", "abc", 3, "abc"},
		{"truncate adds ellipsis", "hello world", 8, "hello..."},
		// maxLen counts runes, so a multi-byte string that fits in runes stays.
		{"non-ascii fits by runes", "héllö", 5, "héllö"},
		// Truncation never splits a multi-byte rune (no mojibake).
		{"non-ascii truncates rune-safe", "héllö wörld", 8, "héllö..."},
		{"cjk truncates rune-safe", "日本語のテキスト", 5, "日本..."},
		// maxLen < 3 leaves no room for the ellipsis: cut to the first runes.
		{"maxLen 0", "hello", 0, ""},
		{"maxLen 1", "hello", 1, "h"},
		{"maxLen 2", "hello", 2, "he"},
		{"maxLen 2 non-ascii", "ééé", 2, "éé"},
		{"maxLen 3 is ellipsis only", "hello", 3, "..."},
		{"negative maxLen", "hello", -1, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Truncate(tt.s, tt.max)
			require.Equal(t, tt.want, got)
			require.True(t, utf8.ValidString(got), "result must stay valid UTF-8")
		})
	}
}

func TestJSEscape(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want string
	}{
		{"empty", "", ""},
		{"plain", "hello", "hello"},
		{"single quote", `a'b`, `a\'b`},
		{"double quote", `a"b`, `a\"b`},
		{"backslash", `a\b`, `a\\b`},
		{"newline", "a\nb", `a\nb`},
		{"both quotes", `say "hi" 'there'`, `say \"hi\" \'there\'`},
		{"script breakout", "</script>", `\u003c/script>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := JSEscape(tt.s)
			require.Equal(t, tt.want, got)
			require.False(t, strings.ContainsAny(got, "\n\r"))
			// A raw "<" must never survive: it could close a surrounding
			// <script> block when the literal is embedded in one.
			require.NotContains(t, got, "<")
		})
	}
}

func TestJSONEscape(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want string
	}{
		{"empty", "", ""},
		{"plain text", "hello world", "hello world"},
		{"backslash", `a\b`, `a\\b`},
		{"quote", `a"b`, `a\"b`},
		{"newline", "a\nb", `a\nb`},
		{"all of the above", "a\\b\"c\nd\re\tf", `a\\b\"c\nd\re\tf`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := JSONEscape(tt.s)
			require.Equal(t, tt.want, got)
			// Sanity: result contains no raw newlines.
			require.False(t, strings.ContainsAny(got, "\n\r"))
		})
	}
}
