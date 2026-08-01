package uikit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAssetsPathIsContentHashed(t *testing.T) {
	p := AssetsPath()
	require.Regexp(t, `^/_uikit/[0-9a-f]{12}/$`, p)
	require.Equal(t, p, AssetsPath(), "hash must be stable across calls")
}

// Layout must reference assets under the exact hashed prefix the handler
// serves — the single-source contract that makes the immutable cache header
// safe.
func TestLayoutEmitsHashedAssetsPrefix(t *testing.T) {
	page := Layout("Title", "<p>body</p>", "dark")
	p := AssetsPath()
	require.Contains(t, page, p+"shoelace/themes/dark.css")
	require.Contains(t, page, p+"shoelace/shoelace-autoloader.js")
	require.Contains(t, page, p+"datastar/datastar.js")
	require.NotContains(t, strings.ReplaceAll(page, p, ""), "/_uikit/",
		"no asset reference may bypass the hashed prefix")
}

// LayoutUnder must prefix every asset URL with the proxy path while leaving
// the mount point (AssetsPath) unprefixed — the proxy strips the prefix
// before the request reaches the app's mux.
func TestLayoutUnderPrefixesAssetURLs(t *testing.T) {
	page := LayoutUnder("/mass.llama-cpp", "Title", "<p>body</p>", "dark")
	p := "/mass.llama-cpp" + AssetsPath()
	require.Contains(t, page, p+"shoelace/themes/dark.css")
	require.Contains(t, page, p+"shoelace/shoelace-autoloader.js")
	require.Contains(t, page, p+"datastar/datastar.js")
	require.NotContains(t, strings.ReplaceAll(page, p, ""), "/_uikit/",
		"no asset reference may bypass the proxy prefix")
}

func TestAssetsHandlerServesUnderHashedPrefix(t *testing.T) {
	mux := http.NewServeMux()
	MountAssets(mux)

	t.Run("hashed URL serves with immutable cache header", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, AssetsPath()+"datastar/datastar.js", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Header().Get("Cache-Control"), "immutable")
		require.NotZero(t, rec.Body.Len())
	})

	t.Run("unhashed legacy URL is not served", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_uikit/datastar/datastar.js", nil))
		require.Equal(t, http.StatusNotFound, rec.Code)
	})
}
