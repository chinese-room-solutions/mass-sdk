package gui

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chinese-room-solutions/mass-sdk/connstore"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *connstore.Store {
	t.Helper()
	s, err := connstore.LoadFrom(filepath.Join(t.TempDir(), "connstore.json"))
	require.NoError(t, err)
	return s
}

// stubValidator returns a Validator that calls fn.
type stubValidator struct{ fn func() error }

func (s stubValidator) Validate(_ context.Context) error { return s.fn() }

// applierState is an in-memory ConnectionApplier for tests.
type applierState struct {
	endpoint   string
	token      string
	httpClient *http.Client
}

func (a *applierState) Token() string                { return a.token }
func (a *applierState) SetBaseURL(e string)          { a.endpoint = e }
func (a *applierState) SetToken(t string)            { a.token = t }
func (a *applierState) SetHTTPClient(h *http.Client) { a.httpClient = h }

func postConnection(t *testing.T, h http.HandlerFunc, jsonBody string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/connection", strings.NewReader(jsonBody)).
		WithContext(context.Background())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// hiddenFlusher wraps a ResponseWriter WITHOUT exposing Flush or Unwrap — the
// middleware mistake that silently kills SSE.
type hiddenFlusher struct{ http.ResponseWriter }

// unwrappingWriter hides Flush but implements Unwrap, the contract
// http.ResponseController follows to reach the underlying Flusher.
type unwrappingWriter struct{ http.ResponseWriter }

func (u unwrappingWriter) Unwrap() http.ResponseWriter { return u.ResponseWriter }

// flushSSE must reach a Flusher through an Unwrap()-implementing wrapper, and
// when the stack truly hides it, log the loud once-per-process warning instead
// of failing silently.
func TestFlushSSE(t *testing.T) {
	t.Run("sees through Unwrap wrappers", func(t *testing.T) {
		rec := httptest.NewRecorder()
		flushSSE(unwrappingWriter{rec}, zerolog.Nop())
		require.True(t, rec.Flushed, "the wrapped recorder must have been flushed")
	})

	t.Run("warns loudly, once, when flushing is impossible", func(t *testing.T) {
		var buf bytes.Buffer
		logger := zerolog.New(&buf)
		w := hiddenFlusher{httptest.NewRecorder()}
		flushSSE(w, logger)
		flushSSE(w, logger)
		require.Equal(t, 1, strings.Count(buf.String(), "SSE flush impossible"),
			"the flush-impossible warning must fire exactly once per process")
		require.Contains(t, buf.String(), `"level":"error"`)
	})
}

// TestConnectionHandler_Verify checks that the Verify handler probes the posted
// connection and reports success — without persisting or touching the live client
// (that's ConnectionSaveHandler's job).
func TestConnectionHandler_Verify(t *testing.T) {
	store := newTestStore(t)
	client := &applierState{}

	var gotEndpoint, gotToken string
	h := ConnectionHandler(ConnectionConfig{
		Store:  store,
		Client: client,
		NewValidator: func(endpoint, token string, hc *http.Client) ValidatorInterface {
			gotEndpoint, gotToken = endpoint, token
			require.NotNil(t, hc)
			return stubValidator{fn: func() error { return nil }}
		},
	})

	rr := postConnection(t, h, `{"appEndpoint":"http://example.test","appToken":"good","appCACert":""}`)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "datastar-patch-signals")
	require.Contains(t, rr.Body.String(), `"appConnOK":true`)
	require.Equal(t, "http://example.test", gotEndpoint)
	require.Equal(t, "good", gotToken)

	// Verify never persists or applies — those happen on autosave.
	_, ok := store.GetConn("http://example.test")
	require.False(t, ok, "verify does not persist")
	require.Empty(t, client.endpoint, "verify does not touch the live client")
}

func TestConnectionHandler_BlankTokenKeepsStored(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.SetConn("http://example.test", connstore.Conn{Token: "kept"}))
	client := &applierState{}

	var gotToken string
	h := ConnectionHandler(ConnectionConfig{
		Store:  store,
		Client: client,
		NewValidator: func(_, token string, _ *http.Client) ValidatorInterface {
			gotToken = token
			return stubValidator{fn: func() error { return nil }}
		},
	})

	// Blank token: the stored token is reused for the probe.
	rr := postConnection(t, h, `{"appEndpoint":"http://example.test","appToken":"","appCACert":""}`)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.Contains(t, rr.Body.String(), `"appConnOK":true`)
	require.Equal(t, "kept", gotToken)
}

// TestConnectionHandler_BlankTokenNewEndpointUsesActive guards the fix for the
// "changed only the URL → Auth token is required" bug: when the form's token is
// blank and the new endpoint has nothing stored, the live client's active token
// is reused so the verify proceeds against the new URL.
func TestConnectionHandler_BlankTokenNewEndpointUsesActive(t *testing.T) {
	store := newTestStore(t)
	client := &applierState{token: "active"} // already connected with this token.

	var gotToken string
	h := ConnectionHandler(ConnectionConfig{
		Store:  store,
		Client: client,
		NewValidator: func(_, token string, _ *http.Client) ValidatorInterface {
			gotToken = token
			return stubValidator{fn: func() error { return nil }}
		},
	})

	// New endpoint, blank token, nothing stored for it → reuse the active token.
	rr := postConnection(t, h, `{"appEndpoint":"http://localhost:36455","appToken":"","appCACert":""}`)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.Contains(t, rr.Body.String(), `"appConnOK":true`)
	require.Equal(t, "active", gotToken)
}

// TestConnectionHandler_TokenlessGateway checks that a blank token with nothing
// stored and no active token still probes — with an empty token — so a gateway
// that doesn't require auth (e.g. a local MASS on loopback) verifies OK. The
// client omits the Authorization header entirely when the token is empty.
func TestConnectionHandler_TokenlessGateway(t *testing.T) {
	store := newTestStore(t)
	client := &applierState{}
	var gotToken string
	var probed bool
	h := ConnectionHandler(ConnectionConfig{
		Store:  store,
		Client: client,
		NewValidator: func(_, token string, _ *http.Client) ValidatorInterface {
			gotToken, probed = token, true
			return stubValidator{fn: func() error { return nil }}
		},
	})

	rr := postConnection(t, h, `{"appEndpoint":"http://example.test","appToken":"","appCACert":""}`)
	require.Contains(t, rr.Body.String(), `"appConnOK":true`)
	require.True(t, probed, "a tokenless connection must still be probed")
	require.Empty(t, gotToken, "the probe runs with an empty token")
}

func TestConnectionHandler_BadCAErrors(t *testing.T) {
	store := newTestStore(t)
	client := &applierState{}
	h := ConnectionHandler(ConnectionConfig{
		Store:  store,
		Client: client,
		NewValidator: func(_, _ string, _ *http.Client) ValidatorInterface {
			return stubValidator{fn: func() error { return nil }}
		},
	})

	rr := postConnection(t, h, `{"appEndpoint":"http://example.test","appToken":"t","appCACert":"/no/such/ca.pem"}`)
	require.Contains(t, rr.Body.String(), `"appConnFail":true`)
}

func TestConnectionHandler_VerifyFailure(t *testing.T) {
	store := newTestStore(t)
	client := &applierState{}
	h := ConnectionHandler(ConnectionConfig{
		Store:  store,
		Client: client,
		NewValidator: func(_, _ string, _ *http.Client) ValidatorInterface {
			return stubValidator{fn: func() error { return errors.New("unauthorized") }}
		},
	})

	rr := postConnection(t, h, `{"appEndpoint":"http://example.test","appToken":"bad","appCACert":""}`)
	require.Contains(t, rr.Body.String(), `"appConnFail":true`)
	require.Empty(t, client.token, "verify never touches the client")
}

// TestConnectionSaveHandler_Persists checks the debounced autosave: it persists the
// posted connection and applies it to the live client, with no probe.
func TestConnectionSaveHandler_Persists(t *testing.T) {
	store := newTestStore(t)
	client := &applierState{}
	h := ConnectionSaveHandler(ConnectionConfig{Store: store, Client: client})

	req := httptest.NewRequest(http.MethodPost, "/api/connection/save",
		strings.NewReader(`{"appEndpoint":"http://example.test","appToken":"good","appCACert":""}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	conn, ok := store.GetConn("http://example.test")
	require.True(t, ok)
	require.Equal(t, "good", conn.Token)
	require.Equal(t, "http://example.test", client.endpoint)
	require.Equal(t, "good", client.token)
	require.NotNil(t, client.httpClient)
}

// TestConnectionSaveHandler_NoURLSkips checks a half-edited form with no URL yet
// is not persisted — it's a normal mid-edit state.
func TestConnectionSaveHandler_NoURLSkips(t *testing.T) {
	store := newTestStore(t)
	client := &applierState{}
	h := ConnectionSaveHandler(ConnectionConfig{Store: store, Client: client})

	req := httptest.NewRequest(http.MethodPost, "/api/connection/save",
		strings.NewReader(`{"appEndpoint":"","appToken":"tok","appCACert":""}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)

	require.Empty(t, client.endpoint)
}

// TestConnectionSaveHandler_TokenlessPersists checks a tokenless gateway (valid
// URL, blank token, nothing stored) is a legitimate config and is persisted.
func TestConnectionSaveHandler_TokenlessPersists(t *testing.T) {
	store := newTestStore(t)
	client := &applierState{}
	h := ConnectionSaveHandler(ConnectionConfig{Store: store, Client: client})

	req := httptest.NewRequest(http.MethodPost, "/api/connection/save",
		strings.NewReader(`{"appEndpoint":"http://example.test","appToken":"","appCACert":""}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	conn, ok := store.GetConn("http://example.test")
	require.True(t, ok, "a tokenless config is persisted")
	require.Empty(t, conn.Token)
	require.Equal(t, "http://example.test", client.endpoint)
	require.Empty(t, client.token)
}

// TestConnectionSaveHandler_BlankTokenKeepsStored checks that changing only the URL
// (blank token) keeps the token already stored for that endpoint.
func TestConnectionSaveHandler_BlankTokenKeepsStored(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.SetConn("http://example.test", connstore.Conn{Token: "kept"}))
	client := &applierState{}
	h := ConnectionSaveHandler(ConnectionConfig{Store: store, Client: client})

	req := httptest.NewRequest(http.MethodPost, "/api/connection/save",
		strings.NewReader(`{"appEndpoint":"http://example.test","appToken":"","appCACert":""}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.Equal(t, "kept", client.token, "the stored token is reused when the form's is blank")
}

func TestApplyStoredConnection(t *testing.T) {
	store := newTestStore(t)
	client := &applierState{}

	// No stored entry: a no-op, no error, nothing applied.
	require.NoError(t, ApplyStoredConnection(store, "http://example.test", client))
	require.Empty(t, client.token)

	// Token only: applied, default HTTP client left in place.
	require.NoError(t, store.SetConn("http://example.test", connstore.Conn{Token: "tok"}))
	require.NoError(t, ApplyStoredConnection(store, "http://example.test", client))
	require.Equal(t, "tok", client.token)
	require.Nil(t, client.httpClient)

	// A broken CA path is surfaced as an error.
	require.NoError(t, store.SetConn("http://example.test", connstore.Conn{Token: "tok", CACert: "/no/such/ca.pem"}))
	require.Error(t, ApplyStoredConnection(store, "http://example.test", client))
}
