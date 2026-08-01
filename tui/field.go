package tui

import (
	"fmt"
	"strconv"
	"strings"
)

// FieldKind is how a field is edited + displayed.
type FieldKind uint8

const (
	FieldText   FieldKind = iota // free text
	FieldSecret                  // free text, displayed masked
	FieldPath                    // a filesystem path; same editing as text
	FieldChoice                  // one of a fixed set, cycled with ←/→
	FieldInt                     // an integer within [Min,Max]
)

// Field is one editable row of the form. Int values are kept as text in Value
// and parsed on validate, so all kinds share one edit path. The caller builds
// the field list (the engine is generic over it).
type Field struct {
	Label   string
	Kind    FieldKind
	Value   string
	Choices []string // FieldChoice only
	// ChoiceIndex is the selected index into Choices. The form derives it from
	// Value on intake (so callers can seed a choice field by Value alone) and
	// CycleChoice keeps the pair in sync afterwards.
	ChoiceIndex int
	Min, Max    int // FieldInt only
}

// CycleChoice advances a FieldChoice by delta (wrapping), syncing Value to the
// selection. No-op on a non-choice or empty-choice field.
func CycleChoice(f *Field, delta int) {
	if f.Kind != FieldChoice || len(f.Choices) == 0 {
		return
	}
	n := len(f.Choices)
	idx := ((f.ChoiceIndex+delta)%n + n) % n // wrap, handling negative delta
	f.ChoiceIndex = idx
	f.Value = f.Choices[idx]
}

// normalizeChoice reconciles a choice field's (Value, ChoiceIndex) pair on form
// intake: a Value found in Choices wins (callers seed by value); otherwise a
// clamped ChoiceIndex drives and Value snaps to a real choice. Without this a
// value-seeded field displays one choice while cycling counts from another, so
// the first ←/→ press re-lands on the shown value — a visible no-op.
func normalizeChoice(f *Field) {
	if f.Kind != FieldChoice || len(f.Choices) == 0 {
		return
	}
	for i, c := range f.Choices {
		if c == f.Value {
			f.ChoiceIndex = i
			return
		}
	}
	if f.ChoiceIndex < 0 || f.ChoiceIndex >= len(f.Choices) {
		f.ChoiceIndex = 0
	}
	f.Value = f.Choices[f.ChoiceIndex]
}

// normalizeChoices applies normalizeChoice to each field; run on every slice
// entering the form (the initial spec fields, OnFieldEdited replacements).
func normalizeChoices(fields []Field) {
	for i := range fields {
		normalizeChoice(&fields[i])
	}
}

// ValidateField returns "" when valid, else a short message to show. The only
// rule is the FieldInt range (everything else is free text, and FieldChoice is
// constrained by construction).
func ValidateField(f Field) string {
	if f.Kind != FieldInt {
		return ""
	}
	rangeMsg := fmt.Sprintf("enter a number between %d and %d", f.Min, f.Max)
	v, err := strconv.Atoi(strings.TrimSpace(f.Value))
	if err != nil || v < f.Min || v > f.Max {
		return rangeMsg
	}
	return ""
}

// firstInvalid returns the index of the first invalid field and true, or
// (0,false) when all are valid.
func firstInvalid(fields []Field) (int, bool) {
	for i := range fields {
		if ValidateField(fields[i]) != "" {
			return i, true
		}
	}
	return 0, false
}

// isSpace reports an in-word separator for word-wise caret motion.
func isSpace(c byte) bool { return c == ' ' || c == '\t' }

// wordLeft lands at the start of the word at/just before pos (Ctrl+Left). pos is
// a byte offset, clamped to [0, len].
func wordLeft(value string, pos int) int {
	if pos > len(value) {
		pos = len(value)
	}
	for pos > 0 && isSpace(value[pos-1]) { // skip trailing spaces
		pos--
	}
	for pos > 0 && !isSpace(value[pos-1]) { // skip the word itself
		pos--
	}
	return pos
}

// wordRight lands at the start of the next word after pos (Ctrl+Right), or
// end-of-string if none.
func wordRight(value string, pos int) int {
	if pos > len(value) {
		pos = len(value)
	}
	for pos < len(value) && !isSpace(value[pos]) { // skip current word
		pos++
	}
	for pos < len(value) && isSpace(value[pos]) { // skip the gap
		pos++
	}
	return pos
}

// displayValue is what a field shows in the value column (secrets masked,
// choices bracketed).
func displayValue(f Field) string {
	switch f.Kind {
	case FieldSecret:
		return strings.Repeat("*", len(f.Value))
	case FieldChoice:
		return "< " + f.Value + " >"
	default:
		return f.Value
	}
}
