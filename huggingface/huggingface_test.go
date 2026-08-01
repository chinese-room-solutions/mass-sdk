package huggingface

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// captureRT records every outbound request URL and answers each with the given
// body, letting a test inspect the query params Search builds without needing a
// seam for the hardcoded apiBase host.
type captureRT struct {
	urls []*url.URL
	body string
}

func (rt *captureRT) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.urls = append(rt.urls, req.URL)
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(rt.body)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

// Regression for the pdf2html case: a repo (KernelPryanic/pdf2html) that holds
// .gguf files but has no "gguf" in its name vanished from results because the
// extension keyword was folded into the free-text `search=` param ("pdf2html
// gguf" matches no repo id). The extension must instead ride the HF `filter=`
// param, leaving the user's query untouched.
func TestSearchFiltersByLibraryTagNotSearchString(t *testing.T) {
	rt := &captureRT{body: "[]"} // empty result set: Search stops after one batch
	c := NewClient(nil, WithHTTPClient(&http.Client{Transport: rt}))

	_, err := c.Search(context.Background(), "pdf2html", SearchOptions{FileExts: []string{".gguf"}})
	require.NoError(t, err)
	require.NotEmpty(t, rt.urls, "Search must issue at least one API request")

	q := rt.urls[0].Query()
	require.Equal(t, "pdf2html", q.Get("search"),
		"the extension keyword must not be appended to the free-text search param")
	require.Contains(t, q["filter"], "gguf",
		"the .gguf extension must be sent as an HF library filter")
}

func TestValidateCursor(t *testing.T) {
	tests := []struct {
		name   string
		cursor string
		wantOK bool
	}{
		{"the Link-header shape", "https://huggingface.co/api/models?cursor=abc&limit=5", true},
		{"plain http downgrade", "http://huggingface.co/api/models?cursor=abc", false},
		{"other host", "https://evil.example.com/api/models", false},
		{"host suffix trick", "https://huggingface.co.evil.example.com/api/models", false},
		{"userinfo trick", "https://huggingface.co@evil.example.com/", false},
		{"not a url", "://nope", false},
		{"relative path", "/api/models?cursor=abc", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCursor(tt.cursor)
			if tt.wantOK {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// A bad cursor must be rejected before any request goes out.
func TestSearchRejectsBadCursor(t *testing.T) {
	_, err := NewClient(nil).Search(context.Background(), "llama", SearchOptions{Cursor: "https://evil.example.com/x"})
	require.ErrorContains(t, err, "cursor")
}

func TestSanitizeRepoID(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		wantOK bool
	}{
		{"plain owner/name", "TheBloke/Llama-2-7B-GGUF", true},
		{"dots and underscores", "unsloth/Qwen2.5_7B-instruct", true},
		{"missing owner", "just-a-name", false},
		{"empty", "", false},
		{"extra separator", "a/b/c", false},
		{"traversal owner", "../etc", false},
		{"traversal name", "owner/..", false},
		{"absolute path", "/etc/passwd", false},
		{"backslash", `owner\name`, false},
		{"leading dot segment", ".hidden/name", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SanitizeRepoID(tt.in)
			if tt.wantOK {
				require.NoError(t, err)
				require.Equal(t, tt.in, got)
			} else {
				require.Error(t, err)
				require.Empty(t, got)
			}
		})
	}
}

func TestRetryAfter(t *testing.T) {
	tests := []struct {
		name             string
		header           string
		wantDelay        time.Duration
		wantWorthWaiting bool
	}{
		{"absent", "", retryAfterDefault, true},
		{"delta seconds", "2", 2 * time.Second, true},
		{"zero seconds", "0", 0, true},
		{"unparseable falls back", "soon", retryAfterDefault, true},
		{"past the cap", "60", 0, false},
		{"date in the past retries at once", "Mon, 02 Jan 2006 15:04:05 GMT", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay, worthWaiting := retryAfter(tt.header)
			require.Equal(t, tt.wantWorthWaiting, worthWaiting)
			require.Equal(t, tt.wantDelay, delay)
		})
	}
}

// A 429 is retried once after the hub's cool-off; a cool-off longer than the cap
// is handed back to the caller instead of blocking on it.
func TestGetRetriesRateLimit(t *testing.T) {
	tests := []struct {
		name         string
		retryAfter   string
		wantStatus   int
		wantRequests int32
	}{
		{"short cool-off is waited out", "1", http.StatusOK, 2},
		{"missing header uses the default", "", http.StatusOK, 2},
		{"long cool-off is surfaced", "600", http.StatusTooManyRequests, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if requests.Add(1) == 1 {
					if tt.retryAfter != "" {
						w.Header().Set("Retry-After", tt.retryAfter)
					}
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(srv.Close)

			c := NewClient(nil, WithHTTPClient(srv.Client()))
			resp, err := c.get(context.Background(), srv.URL)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })

			require.Equal(t, tt.wantStatus, resp.StatusCode)
			require.Equal(t, tt.wantRequests, requests.Load())
		})
	}
}

// A cancelled context stops the cool-off wait rather than sleeping it out.
func TestGetRateLimitHonorsCancellation(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := NewClient(nil, WithHTTPClient(srv.Client())).get(ctx, srv.URL)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, int32(1), requests.Load(), "must not retry after cancellation")
}

// NewClient defaults to a bounded HTTP client; WithHTTPClient and WithToken
// replace the transport and add the Bearer header on every request.
func TestClientOptions(t *testing.T) {
	c := NewClient(nil)
	require.Equal(t, defaultTimeout, c.hc.Timeout, "the default client must carry a sane timeout")

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	custom := &http.Client{Timeout: 5 * time.Second}
	c = NewClient(nil, WithHTTPClient(custom), WithToken("hf_secret"))
	require.Same(t, custom, c.hc, "WithHTTPClient must replace the default client")

	resp, err := c.get(context.Background(), srv.URL)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "Bearer hf_secret", gotAuth)

	// Without WithToken no Authorization header is sent.
	resp, err = NewClient(nil, WithHTTPClient(custom)).get(context.Background(), srv.URL)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Empty(t, gotAuth)
}
