package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

const testETag = `"v1-etag"`

// indexServer serves testIndexYAML with ETag conditional support and counts the
// number of full (200) bodies served.
func indexServer(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	var fullServes int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == testETag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", testETag)
		atomic.AddInt32(&fullServes, 1)
		_, _ = w.Write([]byte(testIndexYAML))
	}))
	t.Cleanup(srv.Close)
	return srv, &fullServes
}

func TestFetchFirstThen304(t *testing.T) {
	srv, fullServes := indexServer(t)
	dir := t.TempDir()
	c := NewClient(srv.URL, dir)

	// First fetch: 200, populates cache.
	res, err := c.Fetch(context.Background())
	require.NoError(t, err)
	require.False(t, res.FromCache)
	require.NotNil(t, res.Index)
	require.EqualValues(t, 1, atomic.LoadInt32(fullServes))

	// Cache files exist.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 2) // body + etag

	// Second fetch: 304, served from cache, no new full serve.
	res2, err := c.Fetch(context.Background())
	require.NoError(t, err)
	require.True(t, res2.FromCache)
	require.False(t, res2.Stale)
	require.EqualValues(t, 1, atomic.LoadInt32(fullServes))
	require.Len(t, res2.Index.Packages, 3)
}

func TestFetchCacheHitOnNetworkError(t *testing.T) {
	srv, _ := indexServer(t)
	dir := t.TempDir()

	// Populate cache against the live server.
	c := NewClient(srv.URL, dir)
	_, err := c.Fetch(context.Background())
	require.NoError(t, err)

	// Point a new client at a dead URL but the same cache dir + same url key...
	// Reuse the SAME url so the cache key matches, but close the server first.
	srv.Close()
	res, err := c.Fetch(context.Background())
	require.NoError(t, err)
	require.True(t, res.FromCache)
	require.True(t, res.Stale)
	require.Len(t, res.Index.Packages, 3)
}

func TestFetchNoCacheNetworkErrorFails(t *testing.T) {
	srv, _ := indexServer(t)
	url := srv.URL
	srv.Close() // dead immediately

	c := NewClient(url, t.TempDir())
	_, err := c.Fetch(context.Background())
	require.Error(t, err)
}

func TestFetchCorruptCache(t *testing.T) {
	srv, fullServes := indexServer(t)
	dir := t.TempDir()
	c := NewClient(srv.URL, dir)

	// Populate, then corrupt the cached body.
	_, err := c.Fetch(context.Background())
	require.NoError(t, err)
	bodyPath, _ := c.cachePaths()
	require.NoError(t, os.WriteFile(bodyPath, []byte("schema_version: 1\npackages: [broken\n"), 0o644))

	// Server 304s (etag matches) but the cache is unparseable, so the client
	// drops the ETag and re-fetches the full body, healing the cache.
	res, err := c.Fetch(context.Background())
	require.NoError(t, err)
	require.False(t, res.FromCache)
	require.Len(t, res.Index.Packages, 3)
	require.EqualValues(t, 2, atomic.LoadInt32(fullServes))
}

func TestFetchSchemaRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("schema_version: 99\npackages: []\n"))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, t.TempDir())
	_, err := c.Fetch(context.Background())
	require.ErrorIs(t, err, ErrUnsupportedSchema)
}

func TestFetchUnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, t.TempDir())
	_, err := c.Fetch(context.Background())
	require.Error(t, err)
}

func TestFetch304WithMissingCacheRefetches(t *testing.T) {
	// Realistic conditional server: 304 only when the matching ETag is sent,
	// otherwise a full 200. Exercises the fetchUnconditional fallback when a 304
	// arrives but the cached body is gone.
	srv, fullServes := indexServer(t)
	dir := t.TempDir()
	c := NewClient(srv.URL, dir)
	_, err := c.Fetch(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 1, atomic.LoadInt32(fullServes))

	// Delete only the cached body, keep the etag. Next fetch sends If-None-Match,
	// server 304s, client has no body ⇒ re-fetches unconditionally (200).
	bodyPath, _ := c.cachePaths()
	require.NoError(t, os.Remove(bodyPath))

	res, err := c.Fetch(context.Background())
	require.NoError(t, err)
	require.False(t, res.FromCache)
	require.Len(t, res.Index.Packages, 3)
	require.EqualValues(t, 2, atomic.LoadInt32(fullServes))
}

func TestFetchCacheKeyIsolation(t *testing.T) {
	// Two urls share a cache dir without colliding.
	dir := t.TempDir()
	c1 := NewClient("https://a.test/index.yml", dir)
	c2 := NewClient("https://b.test/index.yml", dir)
	b1, _ := c1.cachePaths()
	b2, _ := c2.cachePaths()
	require.NotEqual(t, b1, b2)
	require.Equal(t, dir, filepath.Dir(b1))
}

func TestCachedIndex(t *testing.T) {
	srv, fullServes := indexServer(t)
	dir := t.TempDir()
	c := NewClient(srv.URL, dir)

	// Nothing fetched yet: a distinguishable sentinel, no network touched.
	_, err := c.CachedIndex()
	require.ErrorIs(t, err, ErrNoCache)
	require.EqualValues(t, 0, atomic.LoadInt32(fullServes))

	_, err = c.Fetch(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 1, atomic.LoadInt32(fullServes))

	// Served from disk, with the server closed so any request would fail.
	srv.Close()
	idx, err := c.CachedIndex()
	require.NoError(t, err)
	require.Len(t, idx.Packages, 3)
	require.EqualValues(t, 1, atomic.LoadInt32(fullServes))
}

func TestCachedIndexCorrupt(t *testing.T) {
	dir := t.TempDir()
	c := NewClient("https://example.test/index.yml", dir)
	bodyPath, _ := c.cachePaths()
	require.NoError(t, os.WriteFile(bodyPath, []byte("schema_version: 99\npackages: []\n"), 0o600))

	// A cache that exists but is not a usable index is not "no cache".
	_, err := c.CachedIndex()
	require.ErrorIs(t, err, ErrUnsupportedSchema)
	require.NotErrorIs(t, err, ErrNoCache)
}
