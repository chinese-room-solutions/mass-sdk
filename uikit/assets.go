package uikit

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"sync"
)

//go:embed assets
var assetsFS embed.FS

// assetsHash is a short content hash of the embedded asset tree, computed once
// on first use. It cache-busts the asset URLs: AssetsHandler serves with an
// immutable year-long Cache-Control, which is only safe because the URL prefix
// changes whenever the embedded bytes do (a browser that cached the old
// Shoelace/Datastar under an unchanged URL would keep it forever).
var assetsHash = sync.OnceValue(func() string {
	h := sha256.New()
	err := fs.WalkDir(assetsFS, "assets", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, readErr := assetsFS.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		h.Write([]byte(path)) // fs.WalkDir visits in sorted order → deterministic
		h.Write([]byte{0})
		h.Write(b)
		return nil
	})
	if err != nil {
		panic("uikit: embedded assets unreadable: " + err.Error()) // build-time invariant.
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
})

// AssetsPath returns the URL prefix the vendored front-end assets (Shoelace
// and Datastar) are served under. The prefix embeds a content hash of the
// asset tree, so a binary shipping different assets serves them under a new
// URL and stale immutable-cached copies can't survive an upgrade. Layout
// references this, and apps mount AssetsHandler at it (via MountAssets). Kept
// as the single source so the two can't drift.
func AssetsPath() string { return "/_uikit/" + assetsHash() + "/" }

// MountAssets registers the vendored front-end assets on mux. Every app that
// serves pages via Layout must call this — Layout references AssetsPath for
// Shoelace/Datastar, so without it the page loads nothing. Mount it outside any
// auth middleware (the assets are needed to render the login page too):
//
//	mux := http.NewServeMux()
//	uikit.MountAssets(mux)
//	mux.Handle("/", requireAuth(app))
func MountAssets(mux *http.ServeMux) {
	mux.Handle(AssetsPath(), AssetsHandler())
}

// AssetsHandler serves the vendored Shoelace dist and Datastar bundle, so a
// standalone app loads its front-end entirely from its own binary — no CDN, no
// network, instant cold start. Prefer MountAssets; use this directly only if you
// need to wrap or compose the handler.
//
// Immutable long-cache headers are safe here because AssetsPath embeds a hash
// of the asset contents: any change to the embedded files changes the URL.
func AssetsHandler() http.Handler {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic("uikit: embedded assets missing: " + err.Error()) // build-time invariant.
	}
	fileServer := http.StripPrefix(AssetsPath(), http.FileServer(http.FS(sub)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		fileServer.ServeHTTP(w, r)
	})
}
