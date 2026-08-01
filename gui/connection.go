package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chinese-room-solutions/mass-sdk/connstore"
	"github.com/chinese-room-solutions/mass-sdk/masstls"
	"github.com/chinese-room-solutions/mass-sdk/uikit"
	"github.com/rs/zerolog"
)

// ConnectionApplier is the live client surface the connection handler drives:
// it reads the active token (to keep it when the form's token is left blank) and
// sets the endpoint, token, and HTTP client once new settings validate. The
// openai client satisfies it directly (Token/SetBaseURL/SetToken/SetHTTPClient).
type ConnectionApplier interface {
	Token() string
	SetBaseURL(endpoint string)
	SetToken(token string)
	SetHTTPClient(*http.Client)
}

// ConnectionValidatorFactory builds a one-shot validator for a candidate
// (endpoint, token) pair using the supplied HTTP client, so the probe is made
// over the same TLS configuration (custom CA) the settings request asks for.
type ConnectionValidatorFactory func(endpoint, token string, hc *http.Client) ValidatorInterface

// ConnectionConfig configures the connection handlers.
type ConnectionConfig struct {
	// Store persists per-endpoint connection settings. Required.
	Store *connstore.Store
	// Client is the live gateway client updated on save. Required.
	Client ConnectionApplier
	// NewValidator builds a probe validator over a CA-aware HTTP client.
	// Required by ConnectionHandler (verify); unused by ConnectionSaveHandler.
	NewValidator ConnectionValidatorFactory
	// Logger records persistence/probe failures. Optional.
	Logger zerolog.Logger
	// ValidateTimeout caps the verify probe. Defaults to 10s.
	ValidateTimeout time.Duration
}

// connForm is the connection settings posted by the menu's fields (the same shape
// for save and verify).
type connForm struct {
	Endpoint string
	Token    string
	CACert   string
}

// decodeConnForm reads the posted Datastar signals, picking the connection
// fields by the same uikit.SignalApp* names the settings controls bind (the
// body carries the page's full signal set, so unrelated keys are ignored).
// Best-effort: an unreadable body yields an empty form, which resolve rejects.
func decodeConnForm(w http.ResponseWriter, r *http.Request) connForm {
	var signals map[string]any
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&signals)
	str := func(key string) string {
		s, _ := signals[key].(string)
		return s
	}
	return connForm{
		Endpoint: str(uikit.SignalAppEndpoint),
		Token:    str(uikit.SignalAppToken),
		CACert:   str(uikit.SignalAppCACert),
	}
}

// resolvedConn is a normalized, ready-to-use connection: the parsed endpoint, the
// effective token (a blank submission filled from storage or the live client), the
// trimmed CA path, and a CA-aware HTTP client built for it.
type resolvedConn struct {
	endpoint string
	token    string
	caCert   string
	hc       *http.Client
}

// resolve reads and normalizes the posted connection, filling a blank token from
// the one stored for the endpoint, else the live client's active token (so the
// operator can change only the URL or CA without re-pasting the secret), and
// building a CA-aware HTTP client. It returns a human-readable reason on bad
// input (empty/unparseable URL, unloadable CA) for the caller to surface or log.
//
// The token is optional: a gateway that doesn't require auth (e.g. a local MASS
// on loopback) accepts a tokenless connection, and the client omits the
// Authorization header entirely when the token is empty. A blank submission still
// falls back to any stored/active token first, so changing only the URL or CA
// doesn't drop the secret.
func (cfg ConnectionConfig) resolve(body connForm) (resolvedConn, string) {
	endpoint := strings.TrimSpace(body.Endpoint)
	caCert := strings.TrimSpace(body.CACert)
	token := strings.TrimSpace(body.Token)

	if endpoint == "" {
		return resolvedConn{}, "gateway URL is required"
	}
	if _, err := url.Parse(endpoint); err != nil {
		return resolvedConn{}, "invalid URL: " + err.Error()
	}
	if token == "" {
		if existing, ok := cfg.Store.GetConn(endpoint); ok && existing.Token != "" {
			token = existing.Token
		} else {
			token = cfg.Client.Token()
		}
	}
	hc, err := masstls.HTTPClient(caCert)
	if err != nil {
		return resolvedConn{}, "CA certificate: " + err.Error()
	}
	return resolvedConn{endpoint: endpoint, token: token, caCert: caCert, hc: hc}, ""
}

// ConnectionSaveHandler returns the POST api/connection/save handler the settings
// menu's connection fields post to (debounced) on change. It persists the posted
// endpoint/token/CA per endpoint and pushes them into the live client — no probe,
// like the other settings. A bad endpoint or token isn't rejected here; it simply
// fails when the gateway is next used and is fixable in settings. Replies 200 on a
// successful save, or a plain error status the caller can ignore (it's best-effort
// autosave). Verifying the connection is the separate ConnectionHandler.
func ConnectionSaveHandler(cfg ConnectionConfig) http.HandlerFunc {
	if cfg.Store == nil {
		panic("gui.ConnectionSaveHandler: Store is required")
	}
	if cfg.Client == nil {
		panic("gui.ConnectionSaveHandler: Client is required")
	}
	return func(w http.ResponseWriter, r *http.Request) {
		body := decodeConnForm(w, r)
		conn, reason := cfg.resolve(body)
		if reason != "" {
			// Unusable settings mid-edit (e.g. no URL yet, or an unloadable CA path):
			// don't persist a half-config, just no-op so the next debounce can. A blank
			// token is not a reason — a tokenless gateway is a valid config, and resolve
			// reuses any stored/active token first, so nothing is clobbered.
			cfg.Logger.Debug().Str("endpoint", strings.TrimSpace(body.Endpoint)).Msg("connection autosave skipped: " + reason)
			http.Error(w, reason, http.StatusBadRequest)
			return
		}
		if err := cfg.Store.SetConn(conn.endpoint, connstore.Conn{Token: conn.token, CACert: conn.caCert}); err != nil {
			cfg.Logger.Warn().Err(err).Str("endpoint", conn.endpoint).Msg("saving connection")
			http.Error(w, "could not save connection", http.StatusInternalServerError)
			return
		}
		cfg.Client.SetBaseURL(conn.endpoint)
		cfg.Client.SetToken(conn.token)
		cfg.Client.SetHTTPClient(conn.hc)
		w.WriteHeader(http.StatusOK)
	}
}

// ConnectionHandler returns the POST api/connection handler the settings menu's
// "Verify" button posts to (see uikit.ConnectionSection). It probes the posted
// endpoint/token over the requested CA to check the gateway accepts them — it does
// NOT persist (the fields autosave via ConnectionSaveHandler). It replies over
// Datastar SSE: a patch-signals setting appConnOK (reachable) or appConnFail
// (not), which drives the section's green/red badge — no reload, menu stays open.
func ConnectionHandler(cfg ConnectionConfig) http.HandlerFunc {
	if cfg.Store == nil {
		panic("gui.ConnectionHandler: Store is required")
	}
	if cfg.Client == nil {
		panic("gui.ConnectionHandler: Client is required")
	}
	if cfg.NewValidator == nil {
		panic("gui.ConnectionHandler: NewValidator is required")
	}
	if cfg.ValidateTimeout <= 0 {
		cfg.ValidateTimeout = 10 * time.Second
	}

	return func(w http.ResponseWriter, r *http.Request) {
		body := decodeConnForm(w, r)

		setupSSE(w)
		// fail shows the red "Connection failed" badge and logs why — the full
		// reason (often a long gateway/dial error) goes to the app log, not the UI.
		fail := func(reason string) {
			cfg.Logger.Warn().Str("endpoint", strings.TrimSpace(body.Endpoint)).Msg("connection verify failed: " + reason)
			writeConnFail(w, cfg.Logger)
		}

		conn, reason := cfg.resolve(body)
		if reason != "" {
			fail(reason)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), cfg.ValidateTimeout)
		defer cancel()
		if err := cfg.NewValidator(conn.endpoint, conn.token, conn.hc).Validate(ctx); err != nil {
			fail("could not connect: " + err.Error())
			return
		}

		// Success: show the OK badge in place of the button without reloading, so the
		// settings menu stays open.
		writeSSE(w, "datastar-patch-signals",
			`signals {"appConnOK":true,"appConnFail":false,"appConnBusy":false}`)
		flushSSE(w, cfg.Logger)
	}
}

// setupSSE writes the headers for a Datastar SSE response.
func setupSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

// flushUnsupportedOnce gates the flush-impossible warning to once per process —
// it's a property of the app's middleware stack, not of one request.
var flushUnsupportedOnce sync.Once

// flushSSE pushes buffered SSE bytes to the client via http.ResponseController,
// which sees through ResponseWriter wrappers that implement Unwrap(). When the
// stack makes flushing impossible (a wrapper hiding http.Flusher without
// Unwrap — the failure mode that once silently killed every MASS SSE stream),
// it logs ONE loud error instead of degrading silently: the events then only
// arrive when the handler returns, so live UI updates are effectively dead.
func flushSSE(w http.ResponseWriter, logger zerolog.Logger) {
	err := http.NewResponseController(w).Flush()
	if errors.Is(err, http.ErrNotSupported) {
		flushUnsupportedOnce.Do(func() {
			logger.Error().Msg("SSE flush impossible: a ResponseWriter wrapper hides http.Flusher (add an Unwrap() method to the middleware wrapper); live SSE updates will NOT reach the page")
		})
	}
}

// writeSSE writes a single SSE event of the given type. data may contain
// newlines; each line is emitted as its own "data:" field.
func writeSSE(w http.ResponseWriter, event, data string) {
	_, _ = io.WriteString(w, "event: "+event+"\n")
	for line := range strings.SplitSeq(data, "\n") {
		_, _ = io.WriteString(w, "data: "+line+"\n")
	}
	_, _ = io.WriteString(w, "\n")
}

// writeConnFail patches the connection section's signals so the red "Connection
// failed" badge shows (appConnFail), the OK badge is cleared, and the Verify
// button stops spinning (appConnBusy). The reason is logged, not shown.
func writeConnFail(w http.ResponseWriter, logger zerolog.Logger) {
	writeSSE(w, "datastar-patch-signals",
		`signals {"appConnFail":true,"appConnOK":false,"appConnBusy":false}`)
	flushSSE(w, logger)
}

// ApplyStoredConnection seeds the live client from the connection stored for
// endpoint: its token and a CA-aware HTTP client. A missing entry is not an
// error (the client keeps its defaults); only a broken CA path is. Call it at
// startup, after constructing the client, in place of a bare token seed.
func ApplyStoredConnection(store *connstore.Store, endpoint string, client ConnectionApplier) error {
	conn, ok := store.GetConn(endpoint)
	if !ok {
		return nil
	}
	if conn.Token != "" {
		client.SetToken(conn.Token)
	}
	if conn.CACert != "" {
		hc, err := masstls.HTTPClient(conn.CACert)
		if err != nil {
			return fmt.Errorf("applying stored CA for %s: %w", endpoint, err)
		}
		client.SetHTTPClient(hc)
	}
	return nil
}
