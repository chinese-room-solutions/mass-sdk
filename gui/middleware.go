package gui

import (
	"net/http"
	"net/url"
	"strings"

	mass "github.com/chinese-room-solutions/mass-client-go"
	"github.com/chinese-room-solutions/mass-sdk/auth"
)

// RequireAuthConfig configures the auth gate.
type RequireAuthConfig struct {
	Client *mass.Client
	Store  *auth.Store
}

// RequireAuth gates next behind a stored token for the client's current
// endpoint, redirecting to the login page otherwise. The login path and
// /__* routes bypass the gate.
//
// The check is local (do we have a token?). MASS rejects stale tokens at
// the API layer; on 401, app handlers should call client.Validate() and
// redirect to LoginPath themselves.
func RequireAuth(cfg RequireAuthConfig, next http.Handler) http.Handler {
	if cfg.Client == nil {
		panic("gui.RequireAuth: Client is required")
	}
	if cfg.Store == nil {
		panic("gui.RequireAuth: Store is required")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		endpoint := cfg.Client.Endpoint()
		token, ok := cfg.Store.Get(endpoint)
		if !ok || token == "" {
			redirectToLogin(w, r)
			return
		}

		// Make sure the live client is in sync with the store. This handles
		// the first request after process startup where main() built the
		// client without a token.
		if cfg.Client.Token() != token {
			cfg.Client.SetToken(token)
		}

		next.ServeHTTP(w, r)
	})
}

func isPublicPath(p string) bool {
	if p == LoginPath {
		return true
	}
	// Singleton focus endpoint and any /__* routes the app reserves are public
	// — they must not be auth-gated, otherwise a second-instance focus call
	// would bounce through the login page.
	if strings.HasPrefix(p, "/__") {
		return true
	}
	return false
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	q := url.Values{}
	q.Set(returnQueryKey, r.URL.RequestURI())
	http.Redirect(w, r, LoginPath+"?"+q.Encode(), http.StatusSeeOther)
}
