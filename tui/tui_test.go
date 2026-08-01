package tui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// feed returns a `next` closure that yields the given bytes in order, then
// reports exhaustion — modelling the terminal's continuation-byte stream.
func feed(bytes ...byte) func() (byte, bool) {
	i := 0
	return func() (byte, bool) {
		if i >= len(bytes) {
			return 0, false
		}
		b := bytes[i]
		i++
		return b, true
	}
}

func TestParseKeySimple(t *testing.T) {
	tests := []struct {
		name  string
		first byte
		want  KeyType
	}{
		{"enter CR", '\r', KeyEnter},
		{"enter LF", '\n', KeyEnter},
		{"tab", '\t', KeyTab},
		{"del backspace", 0x7f, KeyBackspace},
		{"bs backspace", 0x08, KeyBackspace},
		{"ctrl-c", 0x03, KeyCtrlC},
		{"plain char", 'a', KeyChar},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := parseKey(tt.first, feed())
			require.Equal(t, tt.want, k.Type)
			if tt.want == KeyChar {
				require.Equal(t, tt.first, k.Byte)
			}
		})
	}
}

func TestParseKeyBareEscVsCSI(t *testing.T) {
	// A lone ESC (no continuation) → KeyEsc.
	require.Equal(t, KeyEsc, parseKey(0x1b, feed()).Type)
	// ESC followed by a non-introducer → bare Esc (Alt+key not modelled).
	require.Equal(t, KeyEsc, parseKey(0x1b, feed('x')).Type)
}

func TestParseKeyArrows(t *testing.T) {
	tests := []struct {
		name string
		seq  []byte
		want KeyType
	}{
		{"up", []byte{'[', 'A'}, KeyUp},
		{"down", []byte{'[', 'B'}, KeyDown},
		{"right", []byte{'[', 'C'}, KeyRight},
		{"left", []byte{'[', 'D'}, KeyLeft},
		{"home", []byte{'[', 'H'}, KeyHome},
		{"end", []byte{'[', 'F'}, KeyEnd},
		{"ss3 up", []byte{'O', 'A'}, KeyUp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, parseKey(0x1b, feed(tt.seq...)).Type)
		})
	}
}

func TestParseKeyModifiedAndTilde(t *testing.T) {
	// Ctrl+Right: ESC [ 1 ; 5 C
	require.Equal(t, KeyCtrlRight, parseKey(0x1b, feed('[', '1', ';', '5', 'C')).Type)
	// Ctrl+Left: ESC [ 1 ; 5 D
	require.Equal(t, KeyCtrlLeft, parseKey(0x1b, feed('[', '1', ';', '5', 'D')).Type)
	// Home as ESC [ 1 ~
	require.Equal(t, KeyHome, parseKey(0x1b, feed('[', '1', '~')).Type)
	// End as ESC [ 4 ~
	require.Equal(t, KeyEnd, parseKey(0x1b, feed('[', '4', '~')).Type)
	// Delete (ESC [ 3 ~) is recognised-but-unmodelled → Unknown.
	require.Equal(t, KeyUnknown, parseKey(0x1b, feed('[', '3', '~')).Type)
}

func TestParseKeyMouseReports(t *testing.T) {
	// SGR press/release/wheel (ESC [ < b ; x ; y M/m) — swallowed whole, so the
	// parameter digits can't type into a focused field.
	require.Equal(t, KeyUnknown, parseKey(0x1b, feed([]byte("[<0;10;7M")...)).Type)
	require.Equal(t, KeyUnknown, parseKey(0x1b, feed([]byte("[<0;48;13m")...)).Type)
	require.Equal(t, KeyUnknown, parseKey(0x1b, feed([]byte("[<64;5;5M")...)).Type)

	// The report is consumed exactly: bytes after it stay in the stream.
	countingFeed := func(bytes []byte) (func() (byte, bool), *int) {
		i := 0
		return func() (byte, bool) {
			if i >= len(bytes) {
				return 0, false
			}
			b := bytes[i]
			i++
			return b, true
		}, &i
	}
	next, consumed := countingFeed([]byte("[<0;10;7Mq"))
	require.Equal(t, KeyUnknown, parseKey(0x1b, next).Type)
	require.Equal(t, 9, *consumed) // through 'M'; 'q' left for the next read

	// X10 fallback (ESC [ M + exactly three payload bytes), same property.
	next, consumed = countingFeed([]byte{'[', 'M', 32, 42, 39, 'q'})
	require.Equal(t, KeyUnknown, parseKey(0x1b, next).Type)
	require.Equal(t, 5, *consumed)
}

func TestValidateField(t *testing.T) {
	tests := []struct {
		name  string
		field Field
		valid bool
	}{
		{"text always valid", Field{Kind: FieldText, Value: "anything"}, true},
		{"choice always valid", Field{Kind: FieldChoice, Value: "x"}, true},
		{"int in range", Field{Kind: FieldInt, Value: "50", Min: 1, Max: 100}, true},
		{"int at min", Field{Kind: FieldInt, Value: "1", Min: 1, Max: 100}, true},
		{"int at max", Field{Kind: FieldInt, Value: "100", Min: 1, Max: 100}, true},
		{"int below", Field{Kind: FieldInt, Value: "0", Min: 1, Max: 100}, false},
		{"int above", Field{Kind: FieldInt, Value: "101", Min: 1, Max: 100}, false},
		{"int garbage", Field{Kind: FieldInt, Value: "abc", Min: 1, Max: 100}, false},
		{"int trailing", Field{Kind: FieldInt, Value: "50x", Min: 1, Max: 100}, false},
		{"int empty", Field{Kind: FieldInt, Value: "", Min: 1, Max: 100}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := ValidateField(tt.field)
			require.Equal(t, tt.valid, msg == "")
		})
	}
}

func TestFirstInvalid(t *testing.T) {
	fields := []Field{
		{Kind: FieldText, Value: "ok"},
		{Kind: FieldInt, Value: "999", Min: 1, Max: 100}, // invalid
		{Kind: FieldText, Value: "ok"},
	}
	idx, ok := firstInvalid(fields)
	require.True(t, ok)
	require.Equal(t, 1, idx)

	_, ok = firstInvalid([]Field{{Kind: FieldText, Value: "ok"}})
	require.False(t, ok)
}

func TestCycleChoice(t *testing.T) {
	f := Field{Kind: FieldChoice, Choices: []string{"a", "b", "c"}, ChoiceIndex: 0, Value: "a"}
	CycleChoice(&f, 1)
	require.Equal(t, "b", f.Value)
	require.Equal(t, 1, f.ChoiceIndex)
	CycleChoice(&f, -1)
	require.Equal(t, "a", f.Value)
	// wrap negative
	CycleChoice(&f, -1)
	require.Equal(t, "c", f.Value)
	require.Equal(t, 2, f.ChoiceIndex)
	// wrap positive
	CycleChoice(&f, 1)
	require.Equal(t, "a", f.Value)

	// no-op on non-choice / empty
	g := Field{Kind: FieldText, Value: "x"}
	CycleChoice(&g, 1)
	require.Equal(t, "x", g.Value)
}

func TestWordMotion(t *testing.T) {
	const s = "foo bar  baz"
	// from end
	require.Equal(t, 9, wordLeft(s, len(s)))  // start of "baz"
	require.Equal(t, 4, wordLeft(s, 9))       // start of "bar"
	require.Equal(t, 0, wordLeft(s, 4))       // start of "foo"
	require.Equal(t, 0, wordLeft(s, 0))       // clamp
	// right
	require.Equal(t, 4, wordRight(s, 0))      // past "foo " → start of "bar"
	require.Equal(t, 9, wordRight(s, 4))      // past "bar  " → start of "baz"
	require.Equal(t, len(s), wordRight(s, 9)) // end
}

func TestDisplayValue(t *testing.T) {
	require.Equal(t, "hello", displayValue(Field{Kind: FieldText, Value: "hello"}))
	require.Equal(t, "*****", displayValue(Field{Kind: FieldSecret, Value: "hello"}))
	require.Equal(t, "< opt >", displayValue(Field{Kind: FieldChoice, Value: "opt"}))
	require.Equal(t, "/a/b", displayValue(Field{Kind: FieldPath, Value: "/a/b"}))
}

func TestClip(t *testing.T) {
	require.Equal(t, "abc", clip("abc", 5))
	require.Equal(t, "abc", clip("abc", 3))
	require.Equal(t, "ab…", clip("abcd", 3))
	require.Equal(t, "…", clip("abcd", 1))
}

func TestLowerByte(t *testing.T) {
	require.Equal(t, byte('a'), lowerByte('A'))
	require.Equal(t, byte('z'), lowerByte('Z'))
	require.Equal(t, byte('a'), lowerByte('a'))
	require.Equal(t, byte('5'), lowerByte('5'))
}
