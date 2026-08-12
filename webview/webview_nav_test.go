package webview

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOriginPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "http with port", url: "http://127.0.0.1:8080/app", want: "http://127.0.0.1:8080/"},
		{name: "https keeps scheme", url: "https://app.example/ui", want: "https://app.example/"},
		{name: "no path", url: "http://localhost:1234", want: "http://localhost:1234/"},
		{name: "query and fragment dropped", url: "http://localhost:1234/x?a=1#f", want: "http://localhost:1234/"},
		{name: "file scheme", url: "file:///tmp/index.html", want: ""},
		{name: "no host", url: "http:///app", want: ""},
		{name: "empty", url: "", want: ""},
		{name: "unparsable", url: "http://[::1", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, originPrefix(tt.url))
		})
	}
}

func TestShouldOpenExternally(t *testing.T) {
	t.Parallel()

	const origin = "http://127.0.0.1:8080/"

	tests := []struct {
		name   string
		uri    string
		origin string
		want   bool
	}{
		{name: "same origin page", uri: origin + "settings", origin: origin, want: false},
		{name: "same origin root", uri: origin, origin: origin, want: false},
		{name: "other port is foreign", uri: "http://127.0.0.1:9090/", origin: origin, want: true},
		{name: "foreign https", uri: "https://example.com/docs", origin: origin, want: true},
		{name: "foreign http", uri: "http://example.com/", origin: origin, want: true},
		{name: "mailto", uri: "mailto:someone@example.com", origin: origin, want: true},
		{name: "about blank stays", uri: "about:blank", origin: origin, want: false},
		{name: "data uri stays", uri: "data:text/html,<p>hi</p>", origin: origin, want: false},
		{name: "blob stays", uri: "blob:http://127.0.0.1:8080/abc", origin: origin, want: false},
		{name: "file stays", uri: "file:///etc/passwd", origin: origin, want: false},
		{name: "empty uri", uri: "", origin: origin, want: false},
		{name: "empty origin keeps everything in page", uri: "https://example.com/", origin: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, shouldOpenExternally(tt.uri, tt.origin))
		})
	}
}
