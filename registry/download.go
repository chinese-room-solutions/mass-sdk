package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/KernelPryanic/ctxerr"
)

// Retry policy for a single Download. A runtime package is tens of megabytes,
// so a transport blip near the end of the transfer is the common failure — and
// re-clicking install is the operator's only recourse if we give up. Attempts
// are few and the backoff is short: the failures worth retrying (a dropped
// connection, a 5xx from the asset host) either clear in seconds or not at all.
// Jitter keeps a fleet of workers installing the same runtime from retrying in
// lockstep.
const (
	downloadAttempts    = 3
	downloadBackoffBase = 1 * time.Second
	downloadBackoffMax  = 10 * time.Second
	downloadJitter      = 0.25
)

// Download streams the artifact to destPath, verifying its sha256 before moving
// it into place. It writes to a temp file in destPath's directory, hashes while
// streaming, and only renames on a digest match; on mismatch or any failure the
// temp file is removed and destPath is left untouched. It refuses a placeholder
// ("TBD") digest — an unreleased asset must never install. The download honors
// ctx cancellation mid-stream. destPath's parent directory must exist.
//
// A failed attempt is retried with exponential backoff (see downloadAttempts):
// transport errors and 5xx responses are transient, while a 4xx is terminal —
// the asset host has answered and the answer won't change. Each attempt
// refetches the whole artifact; there is no range resume, so a retry always
// hashes bytes from one single response and can never splice two.
//
// A checksum mismatch gets exactly one refetch. A truncated transfer hashes
// wrong, which is precisely what a retry fixes; but a mismatch that survives a
// clean second fetch means the bytes on the host don't match the index, and
// hammering it won't change that. The second failure returns
// [ErrChecksumMismatch].
//
// The hc argument may be nil, in which case a client with connection-setup
// timeouts but no total timeout is used: artifacts are arbitrarily large, so
// bounding the whole transfer would make big downloads fail on slow links.
// Cancellation is the caller's ctx, which aborts mid-stream.
func Download(ctx context.Context, hc *http.Client, artifact Artifact, destPath string) error {
	if artifact.IsPlaceholder() {
		return ctxerr.With(
			fmt.Errorf("%w: %s", ErrPlaceholderArtifact, artifact.URL),
			map[string]any{"url": artifact.URL},
		)
	}
	if hc == nil {
		hc = &http.Client{Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		}}
	}

	backoff := downloadBackoffBase
	checksumRefetched := false
	for attempt := 1; ; attempt++ {
		retryable, err := downloadOnce(ctx, hc, artifact, destPath)
		switch {
		case err == nil:
			return nil
		case ctx.Err() != nil:
			// The caller gave up; its cancellation is not a transport fault.
			return err
		case errors.Is(err, ErrChecksumMismatch):
			if checksumRefetched {
				return err
			}
			checksumRefetched = true
		case retryable && attempt < downloadAttempts:
			// Fall through to the backoff wait.
		default:
			return err
		}
		if waitErr := wait(ctx, jitter(backoff)); waitErr != nil {
			// Report why the download failed and why we stopped retrying.
			return errors.Join(err, waitErr)
		}
		backoff = min(backoff*2, downloadBackoffMax)
	}
}

// downloadOnce performs one full GET of the artifact into destPath. It reports
// whether the failure is worth another attempt: a transport error or a 5xx is,
// a 4xx or a local filesystem failure is not. A checksum mismatch is reported
// as not retryable — [Download] gives that case its own single refetch.
func downloadOnce(ctx context.Context, hc *http.Client, artifact Artifact, destPath string) (retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return false, ctxerr.With(fmt.Errorf("building artifact request: %w", err), map[string]any{"url": artifact.URL})
	}
	resp, err := hc.Do(req)
	if err != nil {
		return true, ctxerr.With(fmt.Errorf("fetching artifact: %w", err), map[string]any{"url": artifact.URL})
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode >= http.StatusInternalServerError, ctxerr.With(
			fmt.Errorf("fetching artifact: unexpected status %d", resp.StatusCode),
			map[string]any{"url": artifact.URL, "status": resp.StatusCode},
		)
	}

	tmp, err := os.CreateTemp(filepath.Dir(destPath), "."+filepath.Base(destPath)+".*.tmp")
	if err != nil {
		return false, ctxerr.With(fmt.Errorf("creating temp file: %w", err), map[string]any{"dest": destPath})
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName) // best-effort cleanup on any failure.
		}
	}()

	h := sha256.New()
	// ctxReader lets a cancelled context abort the copy even when the transport
	// does not notice it promptly.
	if _, err = io.Copy(io.MultiWriter(tmp, h), &ctxReader{ctx: ctx, r: resp.Body}); err != nil {
		_ = tmp.Close()
		return true, ctxerr.With(fmt.Errorf("streaming artifact: %w", err), map[string]any{"url": artifact.URL})
	}
	if err = tmp.Close(); err != nil {
		return false, ctxerr.With(fmt.Errorf("closing temp file: %w", err), map[string]any{"dest": destPath})
	}

	sum := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(sum, artifact.SHA256) {
		err = ctxerr.With(
			fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, artifact.SHA256, sum),
			map[string]any{"expected": artifact.SHA256, "actual": sum, "url": artifact.URL},
		)
		return false, err
	}

	if err = os.Rename(tmpName, destPath); err != nil {
		return false, ctxerr.With(fmt.Errorf("renaming into place: %w", err), map[string]any{"dest": destPath})
	}
	return false, nil
}

// wait sleeps for d, or returns early with ctx's error if the caller gives up
// first.
func wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// jitter spreads d by ±downloadJitter so concurrent downloads of the same
// artifact don't retry in lockstep.
func jitter(d time.Duration) time.Duration {
	spread := 1 + downloadJitter*(2*rand.Float64()-1)
	return time.Duration(float64(d) * spread)
}

// ctxReader wraps a reader so a cancelled context aborts a blocking Read.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (cr *ctxReader) Read(p []byte) (int, error) {
	if err := cr.ctx.Err(); err != nil {
		return 0, err
	}
	return cr.r.Read(p)
}
