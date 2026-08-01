package uikit

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToInt32(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want int32
	}{
		{"float64 positive", float64(42), 42},
		{"float64 zero", float64(0), 0},
		{"float64 negative", float64(-10), -10},
		{"float64 with fraction truncates", float64(3.9), 3},
		{"string positive", "123", 123},
		{"string zero", "0", 0},
		{"string negative", "-5", -5},
		{"string empty", "", 0},
		{"string non-numeric", "abc", 0},
		{"nil", nil, 0},
		{"int (unsupported type)", int(7), 0},
		{"bool (unsupported type)", true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToInt32(tt.v)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestRenderAlert(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		msg      string
		variant  string
		duration int
		checks   []string // substrings that must appear
	}{
		{
			name:    "empty message returns empty div",
			id:      "alert-1",
			msg:     "",
			variant: "success",
			checks:  []string{`<div id="alert-1"></div>`},
		},
		{
			name:    "basic alert without duration",
			id:      "alert-2",
			msg:     "Saved!",
			variant: "success",
			checks:  []string{`id="alert-2"`, `variant="success"`, `open`, `Saved!`},
		},
		{
			name:     "alert with duration",
			id:       "alert-3",
			msg:      "Error occurred",
			variant:  "danger",
			duration: 5000,
			checks:   []string{`variant="danger"`, `duration="5000"`, `Error occurred`},
		},
		{
			name:    "html-escapes id and message",
			id:      `a"b`,
			msg:     `<script>alert("xss")</script>`,
			variant: "warning",
			checks:  []string{`a&#34;b`, `&lt;script&gt;`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderAlert(tt.id, tt.msg, tt.variant, tt.duration)
			for _, sub := range tt.checks {
				require.True(t, strings.Contains(got, sub),
					"expected %q to contain %q", got, sub)
			}
		})
	}
}

func TestRenderAlert_NoDurationAttr(t *testing.T) {
	got := RenderAlert("x", "msg", "info", 0)
	require.NotContains(t, got, "duration")
}
