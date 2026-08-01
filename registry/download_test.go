package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestDownloadHappyPath(t *testing.T) {
	payload := []byte("the artifact bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)

	dest := filepath.Join(t.TempDir(), "artifact.bin")
	art := Artifact{URL: srv.URL, SHA256: digest(payload)}
	require.NoError(t, Download(context.Background(), srv.Client(), art, dest))

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	require.Equal(t, payload, got)
}

func TestDownloadChecksumMismatch(t *testing.T) {
	payload := []byte("tampered")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	dest := filepath.Join(dir, "artifact.bin")
	art := Artifact{URL: srv.URL, SHA256: digest([]byte("expected different"))}
	err := Download(context.Background(), srv.Client(), art, dest)
	require.ErrorIs(t, err, ErrChecksumMismatch)

	// dest not created; no temp files left behind.
	_, statErr := os.Stat(dest)
	require.True(t, os.IsNotExist(statErr))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestDownloadPlaceholderRefused(t *testing.T) {
	// Server should never be hit.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server must not be contacted for a placeholder artifact")
	}))
	t.Cleanup(srv.Close)

	dest := filepath.Join(t.TempDir(), "artifact.bin")
	art := Artifact{URL: srv.URL, SHA256: "TBD"}
	err := Download(context.Background(), srv.Client(), art, dest)
	require.ErrorIs(t, err, ErrPlaceholderArtifact)
}

func TestDownloadCancellation(t *testing.T) {
	// Stream slowly so cancellation lands mid-copy.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 100; i++ {
			select {
			case <-r.Context().Done():
				return
			default:
			}
			_, _ = w.Write(make([]byte, 10))
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(20 * time.Millisecond)
		}
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	dest := filepath.Join(dir, "artifact.bin")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	art := Artifact{URL: srv.URL, SHA256: digest([]byte("whatever"))}
	err := Download(ctx, srv.Client(), art, dest)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	// No partial dest, no leftover temp.
	_, statErr := os.Stat(dest)
	require.True(t, os.IsNotExist(statErr))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestDownloadUnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	dest := filepath.Join(t.TempDir(), "artifact.bin")
	art := Artifact{URL: srv.URL, SHA256: digest([]byte("x"))}
	require.Error(t, Download(context.Background(), srv.Client(), art, dest))
}

// A failed attempt is retried: a transport blip or a 5xx gets another try, a 4xx
// does not, and a checksum mismatch — what a truncated transfer looks like — gets
// exactly one refetch. Every attempt refetches from zero, so no attempt may send
// a Range header.
func TestDownloadRetry(t *testing.T) {
	good := []byte("the artifact bytes")
	short := good[:5]

	// abort sends a truncated body under a full Content-Length and drops the
	// connection, so the client's copy fails with an unexpected EOF — a transport
	// blip mid-transfer, not a clean short read.
	abort := func(w http.ResponseWriter) {
		w.Header().Set("Content-Length", strconv.Itoa(len(good)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(short)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		panic(http.ErrAbortHandler)
	}

	tests := []struct {
		name string
		// respond writes the response for the given 1-based attempt.
		respond      func(w http.ResponseWriter, attempt int)
		wantContent  []byte // non-nil: expect success with these bytes.
		wantErrIs    error
		wantAttempts int32
	}{
		{
			name: "transport blip then success",
			respond: func(w http.ResponseWriter, attempt int) {
				if attempt == 1 {
					abort(w)
					return
				}
				_, _ = w.Write(good)
			},
			wantContent:  good,
			wantAttempts: 2,
		},
		{
			name: "server error then success",
			respond: func(w http.ResponseWriter, attempt int) {
				if attempt == 1 {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				_, _ = w.Write(good)
			},
			wantContent:  good,
			wantAttempts: 2,
		},
		{
			name: "truncated body refetched once then verifies",
			respond: func(w http.ResponseWriter, attempt int) {
				if attempt == 1 {
					_, _ = w.Write(short) // clean EOF, wrong digest.
					return
				}
				_, _ = w.Write(good)
			},
			wantContent:  good,
			wantAttempts: 2,
		},
		{
			name: "persistent checksum mismatch refetched only once",
			respond: func(w http.ResponseWriter, _ int) {
				_, _ = w.Write(short)
			},
			wantErrIs:    ErrChecksumMismatch,
			wantAttempts: 2,
		},
		{
			name: "server error retried up to the attempt cap",
			respond: func(w http.ResponseWriter, _ int) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			wantAttempts: downloadAttempts,
		},
		{
			name: "client error is terminal",
			respond: func(w http.ResponseWriter, _ int) {
				w.WriteHeader(http.StatusNotFound)
			},
			wantAttempts: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel() // each case owns its server, counter, and temp dir.
			var attempts atomic.Int32
			var sawRange atomic.Bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Range") != "" {
					sawRange.Store(true)
				}
				tt.respond(w, int(attempts.Add(1)))
			}))
			t.Cleanup(srv.Close)

			dir := t.TempDir()
			dest := filepath.Join(dir, "artifact.bin")
			art := Artifact{URL: srv.URL, SHA256: digest(good)}
			err := Download(context.Background(), srv.Client(), art, dest)

			require.False(t, sawRange.Load(), "every attempt must refetch from zero")
			require.Equal(t, tt.wantAttempts, attempts.Load(), "attempt count")
			if tt.wantContent != nil {
				require.NoError(t, err)
				got, readErr := os.ReadFile(dest)
				require.NoError(t, readErr)
				require.Equal(t, tt.wantContent, got)
				return
			}

			require.Error(t, err)
			if tt.wantErrIs != nil {
				require.ErrorIs(t, err, tt.wantErrIs)
			}
			// A failed download leaves destPath untouched and no temp behind.
			_, statErr := os.Stat(dest)
			require.True(t, os.IsNotExist(statErr))
			entries, readErr := os.ReadDir(dir)
			require.NoError(t, readErr)
			require.Empty(t, entries)
		})
	}
}

// A cancelled context stops the retry loop instead of sleeping out the backoff.
func TestDownloadCancellationStopsRetrying(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	// Enough time for the first attempt, far less than the first backoff.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	dest := filepath.Join(t.TempDir(), "artifact.bin")
	art := Artifact{URL: srv.URL, SHA256: digest([]byte("x"))}
	err := Download(ctx, srv.Client(), art, dest)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, int32(1), attempts.Load(), "must not attempt again after cancellation")
}

func TestDownloadNilClient(t *testing.T) {
	payload := []byte("default client works")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)

	dest := filepath.Join(t.TempDir(), "artifact.bin")
	art := Artifact{URL: srv.URL, SHA256: digest(payload)}
	require.NoError(t, Download(context.Background(), nil, art, dest))
}
