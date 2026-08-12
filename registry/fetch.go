package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/KernelPryanic/ctxerr"
	"github.com/chinese-room-solutions/mass-sdk/fsutil"
)

// defaultTimeout caps a single index fetch when the caller does not supply its
// own http.Client — an unbounded request against the raw-content host would
// hang the whole search.
const defaultTimeout = 30 * time.Second

// Client fetches the index from url, caching the response under cacheDir with
// ETag/If-None-Match conditional revalidation.
type Client struct {
	url      string
	cacheDir string
	hc       *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the default http.Client (e.g. for tests or a custom
// transport). Passing nil is ignored.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.hc = hc
		}
	}
}

// NewClient returns a Client that fetches the index from url and caches it under
// cacheDir. cacheDir is created on first fetch if absent.
func NewClient(url, cacheDir string, opts ...Option) *Client {
	c := &Client{
		url:      url,
		cacheDir: cacheDir,
		hc:       &http.Client{Timeout: defaultTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// FetchResult is the outcome of a Fetch.
type FetchResult struct {
	// Index is the parsed index.
	Index *Index
	// FromCache is true when the index came from the on-disk cache rather than a
	// fresh 200 response — either a 304 (cache still current) or a network
	// failure that fell back to a cached copy.
	FromCache bool
	// Stale is true when FromCache is set because the network was unreachable and
	// the cached copy could not be revalidated, so it may be out of date.
	Stale bool
}

// cachePaths derives the body and etag file paths for the client's url. The key
// is a hash of the url so multiple registries can share one cacheDir.
func (c *Client) cachePaths() (body, etag string) {
	sum := sha256.Sum256([]byte(c.url))
	key := hex.EncodeToString(sum[:])
	return filepath.Join(c.cacheDir, key+".yml"), filepath.Join(c.cacheDir, key+".etag")
}

// Fetch retrieves the index, using ETag conditional revalidation against the
// on-disk cache. On a 304 it serves the cached body. If the request fails at the
// network level and a cached copy exists, it serves the cache and marks the
// result stale. A corrupt cache is treated as absent.
func (c *Client) Fetch(ctx context.Context) (*FetchResult, error) {
	bodyPath, etagPath := c.cachePaths()
	cachedBody, cachedETag := c.readCache(bodyPath, etagPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, ctxerr.With(fmt.Errorf("building index request: %w", err), map[string]any{"url": c.url})
	}
	if cachedETag != "" {
		req.Header.Set("If-None-Match", cachedETag)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		// Context cancellation is the caller's decision — never mask it with cache.
		if ctx.Err() != nil {
			return nil, ctxerr.With(fmt.Errorf("fetching index: %w", err), map[string]any{"url": c.url})
		}
		if cachedBody != nil {
			idx, perr := ParseIndex(cachedBody)
			if perr == nil {
				return &FetchResult{Index: idx, FromCache: true, Stale: true}, nil
			}
		}
		return nil, ctxerr.With(fmt.Errorf("fetching index: %w", err), map[string]any{"url": c.url})
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusNotModified:
		if cachedBody != nil {
			if idx, err := ParseIndex(cachedBody); err == nil {
				return &FetchResult{Index: idx, FromCache: true}, nil
			}
		}
		// No cache, or a corrupt/unparseable one: the ETag is stale relative to
		// what we can serve, so drop it and re-fetch the full body.
		return c.fetchUnconditional(ctx)

	case http.StatusOK:
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, ctxerr.With(fmt.Errorf("reading index body: %w", err), map[string]any{"url": c.url})
		}
		idx, err := ParseIndex(data)
		if err != nil {
			return nil, err
		}
		c.writeCache(bodyPath, etagPath, data, resp.Header.Get("ETag"))
		return &FetchResult{Index: idx}, nil

	default:
		return nil, ctxerr.With(
			fmt.Errorf("fetching index: unexpected status %d", resp.StatusCode),
			map[string]any{"url": c.url, "status": resp.StatusCode},
		)
	}
}

// CachedIndex returns the index from the on-disk cache without any network I/O,
// for callers that must not block on the network (the hub's Register compat
// check). It returns ErrNoCache when nothing has been fetched yet, and a parse
// error when the cached copy is unreadable as an index.
func (c *Client) CachedIndex() (*Index, error) {
	bodyPath, _ := c.cachePaths()
	data, err := os.ReadFile(bodyPath)
	if err != nil {
		return nil, ctxerr.With(fmt.Errorf("%w: %s", ErrNoCache, bodyPath), map[string]any{"url": c.url, "path": bodyPath})
	}
	return ParseIndex(data)
}

// fetchUnconditional re-fetches without an If-None-Match header. Used when the
// server returns 304 but the local cache has vanished.
func (c *Client) fetchUnconditional(ctx context.Context) (*FetchResult, error) {
	bodyPath, etagPath := c.cachePaths()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, ctxerr.With(fmt.Errorf("building index request: %w", err), map[string]any{"url": c.url})
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, ctxerr.With(fmt.Errorf("fetching index: %w", err), map[string]any{"url": c.url})
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, ctxerr.With(
			fmt.Errorf("fetching index: unexpected status %d", resp.StatusCode),
			map[string]any{"url": c.url, "status": resp.StatusCode},
		)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ctxerr.With(fmt.Errorf("reading index body: %w", err), map[string]any{"url": c.url})
	}
	idx, err := ParseIndex(data)
	if err != nil {
		return nil, err
	}
	c.writeCache(bodyPath, etagPath, data, resp.Header.Get("ETag"))
	return &FetchResult{Index: idx}, nil
}

// readCache loads the cached body and etag. A missing or unreadable body is
// treated as no cache (nil body); an etag without a body is ignored.
func (c *Client) readCache(bodyPath, etagPath string) (body []byte, etag string) {
	data, err := os.ReadFile(bodyPath)
	if err != nil {
		return nil, ""
	}
	if e, err := os.ReadFile(etagPath); err == nil {
		etag = string(e)
	}
	return data, etag
}

// writeCache persists the body and etag atomically. Cache writes are
// best-effort: a failure to cache must not fail an otherwise-good fetch.
func (c *Client) writeCache(bodyPath, etagPath string, data []byte, etag string) {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return
	}
	if err := fsutil.WriteFileAtomic(bodyPath, data, 0o644); err != nil {
		return
	}
	if etag != "" {
		_ = fsutil.WriteFileAtomic(etagPath, []byte(etag), 0o644)
	} else {
		_ = os.Remove(etagPath)
	}
}
