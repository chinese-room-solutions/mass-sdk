package gui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	mass "github.com/chinese-room-solutions/mass-client-go"
	"github.com/chinese-room-solutions/mass-sdk/auth"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *auth.Store {
	t.Helper()
	s, err := auth.LoadFrom(filepath.Join(t.TempDir(), "auth.json"))
	require.NoError(t, err)
	return s
}

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
	client := &mass.Client{}
	client.SetEndpoint("http://localhost:3455")

	called := false
	mw := RequireAuth(RequireAuthConfig{Client: client, Store: store},
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))

	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/protected", nil))

	require.False(t, called, "downstream handler must not run without token")
	require.Equal(t, http.StatusSeeOther, rr.Code)
	require.Contains(t, rr.Header().Get("Location"), LoginPath)
	require.Contains(t, rr.Header().Get("Location"), "return=%2Fprotected")
}

func TestRequireAuth_TokenInStorePassesThrough(t *testing.T) {
	store := newTestStore(t)
	client := &mass.Client{}
	client.SetEndpoint("http://localhost:3455")
	require.NoError(t, store.Set("http://localhost:3455", "tokA"))

	called := false
	mw := RequireAuth(RequireAuthConfig{Client: client, Store: store},
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))

	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/protected", nil))

	require.True(t, called)
	require.Equal(t, "tokA", client.Token(), "middleware must rehydrate client token from store")
}

func TestRequireAuth_PublicPathsBypass(t *testing.T) {
	store := newTestStore(t)
	client := &mass.Client{}

	called := false
	mw := RequireAuth(RequireAuthConfig{Client: client, Store: store},
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))

	for _, p := range []string{LoginPath, "/__focus", "/__anything"} {
		called = false
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p, nil))
		require.True(t, called, "public path must pass through: %s", p)
	}
}

func TestLoginHandler_PostSuccessRedirects(t *testing.T) {
	// Stand up a fake MASS that accepts ListModels with the right token.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mass.v1.Mass/ListModels", r.URL.Path)
		require.Equal(t, "Bearer good-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	store := newTestStore(t)
	client := &mass.Client{HTTPClient: srv.Client()}

	h := LoginHandler(LoginConfig{
		Client:        client,
		Store:         store,
		DefaultReturn: "/home",
		AppTitle:      "Test",
	})

	body := strings.NewReader(`{"mass_endpoint":"` + srv.URL + `","mass_token":"good-token","mass_return":"/home"}`)
	req := httptest.NewRequest(http.MethodPost, LoginPath, body).WithContext(context.Background())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.Contains(t, rr.Body.String(), "datastar-execute-script")
	require.Contains(t, rr.Body.String(), "/home")

	// Token should now be persisted in the store and live on the client.
	tok, ok := store.Get(srv.URL)
	require.True(t, ok)
	require.Equal(t, "good-token", tok)
	require.Equal(t, "good-token", client.Token())
}

func TestLoginHandler_PostFailureRendersError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	store := newTestStore(t)
	client := &mass.Client{HTTPClient: srv.Client()}

	h := LoginHandler(LoginConfig{
		Client: client, Store: store, DefaultReturn: "/", AppTitle: "Test",
	})

	body := strings.NewReader(`{"mass_endpoint":"` + srv.URL + `","mass_token":"bad","mass_return":"/"}`)
	req := httptest.NewRequest(http.MethodPost, LoginPath, body)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	require.Contains(t, rr.Body.String(), "datastar-patch-elements")
	require.Contains(t, rr.Body.String(), "Could not connect")
	_, ok := store.Get(srv.URL)
	require.False(t, ok, "token must not be persisted on failure")
}
