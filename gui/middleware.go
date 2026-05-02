package gui

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/chinese-room-solutions/mass-sdk/auth"
)

// ValidatorInterface is the minimal interface the GUI auth gate needs from
// whatever client an app uses to talk to its backend. Implementations call
// a known auth-protected endpoint and return nil iff the configured token
// is accepted.
type ValidatorInterface interface {
	Validate(ctx context.Context) error
}

// EndpointFunc returns the current endpoint URL the app's client targets.
// Used as the auth-store key — tokens are scoped per endpoint.
type EndpointFunc func() string

// SetTokenFunc updates the live client's token. Pass empty string to clear.
type SetTokenFunc func(token string)

// RequireAuthConfig configures the auth gate.
type RequireAuthConfig struct {
	// Endpoint returns the live endpoint URL; used as the auth-store key.
	Endpoint EndpointFunc
	// SetToken pushes the stored token into the live client. Optional —
	// if nil, callers are expected to seed the token themselves at startup
	// and the middleware only enforces presence.
	SetToken SetTokenFunc
	// Store holds endpoint→token mappings. Required.
	Store *auth.Store
}

// RequireAuth gates next behind a stored token for the current endpoint,
// redirecting to the login page otherwise. The login path and /__* routes
// bypass the gate.
//
// The check is local (do we have a token?). The backend rejects stale tokens
// at the API layer; on 401, app handlers should call Validator.Validate()
// and redirect to LoginPath themselves.
func RequireAuth(cfg RequireAuthConfig, next http.Handler) http.Handler {
	if cfg.Endpoint == nil {
		panic("gui.RequireAuth: Endpoint is required")
	}
	if cfg.Store == nil {
		panic("gui.RequireAuth: Store is required")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		endpoint := cfg.Endpoint()
		token, ok := cfg.Store.Get(endpoint)
		if !ok || token == "" {
			redirectToLogin(w, r)
			return
		}

		// Make sure the live client is in sync with the store. Handles the
		// first request after process startup where main() built the client
		// without a token.
		if cfg.SetToken != nil {
			cfg.SetToken(token)
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
