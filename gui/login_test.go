package gui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chinese-room-solutions/mass-sdk/auth"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *auth.Store {
	t.Helper()
	s, err := auth.LoadFrom(filepath.Join(t.TempDir(), "auth.json"))
	require.NoError(t, err)
	return s
}

// liveState is a tiny in-memory client stand-in for tests: holds an endpoint
// + token, exposes the func adapters [middleware.go] / [login.go] expect.
type liveState struct {
	endpoint string
	token    string
}

func (s *liveState) getEndpoint() string        { return s.endpoint }
func (s *liveState) setEndpoint(url string)     { s.endpoint = url }
func (s *liveState) setToken(t string)          { s.token = t }
func (s *liveState) getToken() string           { return s.token }

// stubValidator returns a Validator that calls validateFn.
type stubValidator struct{ fn func() error }

func (s stubValidator) Validate(_ context.Context) error { return s.fn() }

func TestRenderLoginPage_ContainsFields(t *testing.T) {
	page := RenderLoginPage(LoginPageData{
		AppTitle:    "Playground",
		EndpointURL: "http://localhost:3455",
		ReturnURL:   "/dashboard",
	})
	require.Contains(t, page, "Playground")
	require.Contains(t, page, `data-bind="mass_endpoint"`)
	require.Contains(t, page, `data-bind="mass_token"`)
	require.Contains(t, page, "http://localhost:3455")
	require.Contains(t, page, "/dashboard")
	require.Contains(t, page, LoginPath)
}

func TestRenderLoginPage_RendersError(t *testing.T) {
	page := RenderLoginPage(LoginPageData{
		AppTitle: "X",
		ErrorMsg: "boom",
	})
	require.Contains(t, page, "boom")
	require.Contains(t, page, `variant="danger"`)
}

func TestExtractBody(t *testing.T) {
	page := "<html><head></head><body class=\"x\">CONTENT</body></html>"
	require.Equal(t, "CONTENT", extractBody(page))
}

func TestRequireAuth_NoTokenRedirects(t *testing.T) {
	store := newTestStore(t)
	state := &liveState{endpoint: "http://localhost:3455"}

	called := false
	mw := RequireAuth(RequireAuthConfig{
		Endpoint: state.getEndpoint,
		SetToken: state.setToken,
		Store:    store,
	}, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true }))

	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/protected", nil))

	require.False(t, called, "downstream handler must not run without token")
	require.Equal(t, http.StatusSeeOther, rr.Code)
	require.Contains(t, rr.Header().Get("Location"), LoginPath)
	require.Contains(t, rr.Header().Get("Location"), "return=%2Fprotected")
}

func TestRequireAuth_TokenInStorePassesThrough(t *testing.T) {
	store := newTestStore(t)
	state := &liveState{endpoint: "http://localhost:3455"}
	require.NoError(t, store.Set("http://localhost:3455", "tokA"))

	called := false
	mw := RequireAuth(RequireAuthConfig{
		Endpoint: state.getEndpoint,
		SetToken: state.setToken,
		Store:    store,
	}, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true }))

	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/protected", nil))

	require.True(t, called)
	require.Equal(t, "tokA", state.getToken(), "middleware must rehydrate client token from store")
}

func TestRequireAuth_PublicPathsBypass(t *testing.T) {
	store := newTestStore(t)
	state := &liveState{}

	called := false
	mw := RequireAuth(RequireAuthConfig{
		Endpoint: state.getEndpoint,
		Store:    store,
	}, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true }))

	for _, p := range []string{LoginPath, "/__focus", "/__anything"} {
		called = false
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p, nil))
		require.True(t, called, "public path must pass through: %s", p)
	}
}

func TestLoginHandler_PostSuccessRedirects(t *testing.T) {
	store := newTestStore(t)
	state := &liveState{}

	h := LoginHandler(LoginConfig{
		Endpoint:    state.getEndpoint,
		SetEndpoint: state.setEndpoint,
		SetToken:    state.setToken,
		NewValidator: func(endpoint, token string) ValidatorInterface {
			require.Equal(t, "http://example.test", endpoint)
			require.Equal(t, "good-token", token)
			return stubValidator{fn: func() error { return nil }}
		},
		Store:         store,
		DefaultReturn: "/home",
		AppTitle:      "Test",
	})

	body := strings.NewReader(`{"mass_endpoint":"http://example.test","mass_token":"good-token","mass_return":"/home"}`)
	req := httptest.NewRequest(http.MethodPost, LoginPath, body).WithContext(context.Background())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.Contains(t, rr.Body.String(), "datastar-execute-script")
	require.Contains(t, rr.Body.String(), "/home")

	tok, ok := store.Get("http://example.test")
	require.True(t, ok)
	require.Equal(t, "good-token", tok)
	require.Equal(t, "good-token", state.getToken())
	require.Equal(t, "http://example.test", state.getEndpoint())
}

func TestLoginHandler_PostFailureRendersError(t *testing.T) {
	store := newTestStore(t)
	state := &liveState{}

	h := LoginHandler(LoginConfig{
		Endpoint:    state.getEndpoint,
		SetEndpoint: state.setEndpoint,
		SetToken:    state.setToken,
		NewValidator: func(_, _ string) ValidatorInterface {
			return stubValidator{fn: func() error { return errors.New("unauthorized") }}
		},
		Store:         store,
		DefaultReturn: "/",
		AppTitle:      "Test",
	})

	body := strings.NewReader(`{"mass_endpoint":"http://example.test","mass_token":"bad","mass_return":"/"}`)
	req := httptest.NewRequest(http.MethodPost, LoginPath, body)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Contains(t, rr.Body.String(), "datastar-patch-elements")
	require.Contains(t, rr.Body.String(), "Could not connect")
	_, ok := store.Get("http://example.test")
	require.False(t, ok, "token must not be persisted on failure")
}
