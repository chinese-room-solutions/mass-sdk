package selfupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// tagServer answers /releases/latest with a redirect to tag, or with status
// when tag is empty.
func tagServer(t *testing.T, tag string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if tag == "" {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Location", "https://example.invalid/owner/repo/releases/tag/"+tag)
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCheckerCheck(t *testing.T) {
	tests := []struct {
		name      string
		current   string
		tag       string
		status    int
		available string
		wantErr   bool
	}{
		{name: "newer release", current: "v0.4.0", tag: "v0.4.2", available: "v0.4.2"},
		{name: "same release", current: "v0.4.2", tag: "v0.4.2"},
		{name: "older release", current: "v0.4.3", tag: "v0.4.2"},
		{name: "dev build opts out", current: "v0.4.0-3-gabc", tag: "v0.4.2"},
		{name: "no published release", current: "v0.4.0", status: http.StatusOK, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := tagServer(t, tc.tag, tc.status)
			c := &Checker{Version: tc.current, BaseURL: srv.URL, Logger: zerolog.Nop()}

			got, err := c.Check(t.Context())
			if tc.wantErr {
				require.Error(t, err)
				require.NotEmpty(t, got.Err)
			} else {
				require.NoError(t, err)
				require.Empty(t, got.Err)
			}
			require.Equal(t, tc.available, got.Available)
			require.Equal(t, tc.available, c.Available())
			require.Equal(t, tc.current, c.Status().Version)
			require.False(t, got.CheckedAt.IsZero())
		})
	}
}

// A failed check must not retract a release already found: the release didn't
// disappear because the network did.
func TestCheckerKeepsFoundTagAcrossFailure(t *testing.T) {
	c := &Checker{Version: "v0.4.0", Logger: zerolog.Nop()}
	c.record("v0.4.2", nil)

	unreachable := httptest.NewServer(nil)
	unreachable.Close()
	c.BaseURL = unreachable.URL

	got, err := c.Check(t.Context())
	require.Error(t, err)
	require.Equal(t, "v0.4.2", got.Available)
	require.NotEmpty(t, got.Err)
}

// OnFound fires once for a newly found tag, not on every re-confirmation.
func TestCheckerOnFound(t *testing.T) {
	var calls atomic.Int32
	var seen atomic.Value
	srv := tagServer(t, "v0.4.2", 0)
	c := &Checker{Version: "v0.4.0", BaseURL: srv.URL, Logger: zerolog.Nop(), OnFound: func(tag string) {
		calls.Add(1)
		seen.Store(tag)
	}}

	_, err := c.Check(t.Context())
	require.NoError(t, err)
	_, err = c.Check(t.Context())
	require.NoError(t, err)

	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, "v0.4.2", seen.Load())
}

// Run checks immediately rather than waiting out the first interval, and stops
// with its context.
func TestCheckerRunChecksImmediatelyAndStops(t *testing.T) {
	srv := tagServer(t, "v0.4.2", 0)
	c := &Checker{Version: "v0.4.0", BaseURL: srv.URL, Interval: time.Hour, Logger: zerolog.Nop()}

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); c.Run(ctx) }()

	require.Eventually(t, func() bool { return c.Available() == "v0.4.2" }, 5*time.Second, 10*time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}
}
