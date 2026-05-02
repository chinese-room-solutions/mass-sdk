package format

import (
	"strings"
	"testing"

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
		{"below KB", 1023, "1023 B"},
		{"exactly 1 KB", 1024, "1.0 KB"},
		{"1.5 KB", 1536, "1.5 KB"},
		{"below MB", 1<<20 - 1, "1024.0 KB"},
		{"exactly 1 MB", 1 << 20, "1.0 MB"},
		{"500 MB", 500 << 20, "500.0 MB"},
		{"below GB", 1<<30 - 1, "1024.0 MB"},
		{"exactly 1 GB", 1 << 30, "1.0 GB"},
		{"4.2 GB", 4509715660, "4.2 GB"},
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, Truncate(tt.s, tt.max))
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
