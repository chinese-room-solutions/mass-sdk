// Package gui provides shared HTTP machinery for MASS app GUIs: a login
// page that captures and validates an endpoint+token (persisted via auth),
// and a middleware that gates app routes behind a valid stored token.
package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	mass "github.com/chinese-room-solutions/mass-client-go"
	"github.com/chinese-room-solutions/mass-sdk/auth"
	"github.com/chinese-room-solutions/mass-sdk/uikit"
)

// LoginPath is the standard mount point for the login handler.
const LoginPath = "/__mass_login"

// returnQueryKey is the query parameter that carries the post-login redirect.
const returnQueryKey = "return"

// Theme controls the rendered login page theme. Defaults to "dark" if empty.
type Theme string

const (
	ThemeDark  Theme = "dark"
	ThemeLight Theme = "light"
)

// LoginConfig configures the login handler.
type LoginConfig struct {
	// Client is the live mass.Client to update on successful login.
	Client *mass.Client
	// Store persists the token keyed by endpoint URL.
	Store *auth.Store
	// DefaultReturn is the post-login URL when no ?return= is provided.
	DefaultReturn string
	// AppTitle is shown in the login page heading.
	AppTitle string
	// Theme controls the page theme.
	Theme Theme
	// ValidateTimeout caps how long Validate() may run. Defaults to 10s.
	ValidateTimeout time.Duration
}

// LoginHandler returns an http.HandlerFunc serving GET (render the form) and
// POST (Datastar SSE: validate + persist + redirect or re-render with error).
func LoginHandler(cfg LoginConfig) http.HandlerFunc {
	if cfg.Client == nil {
		panic("gui.LoginHandler: Client is required")
	}
	if cfg.Store == nil {
		panic("gui.LoginHandler: Store is required")
	}
	if cfg.DefaultReturn == "" {
		cfg.DefaultReturn = "/"
	}
	if cfg.AppTitle == "" {
		cfg.AppTitle = "MASS App"
	}
	if cfg.Theme == "" {
		cfg.Theme = ThemeDark
	}
	if cfg.ValidateTimeout <= 0 {
		cfg.ValidateTimeout = 10 * time.Second
	}

	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleLoginGet(w, r, cfg)
		case http.MethodPost:
			handleLoginPost(w, r, cfg)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleLoginGet(w http.ResponseWriter, r *http.Request, cfg LoginConfig) {
	ret := r.URL.Query().Get(returnQueryKey)
	if ret == "" {
		ret = cfg.DefaultReturn
	}
	page := RenderLoginPage(LoginPageData{
		AppTitle:    cfg.AppTitle,
		EndpointURL: cfg.Client.Endpoint(),
		ReturnURL:   ret,
		Theme:       cfg.Theme,
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, page)
}

func handleLoginPost(w http.ResponseWriter, r *http.Request, cfg LoginConfig) {
	// Datastar sends signal values as a JSON body keyed by signal name.
	var signals struct {
		Endpoint  string `json:"mass_endpoint"`
		Token     string `json:"mass_token"`
		ReturnURL string `json:"mass_return"`
	}
	body, _ := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<14))
	if len(body) > 0 {
		_ = json.Unmarshal(body, &signals)
	}

	signals.Endpoint = strings.TrimSpace(signals.Endpoint)
	signals.Token = strings.TrimSpace(signals.Token)
	if signals.ReturnURL == "" {
		signals.ReturnURL = cfg.DefaultReturn
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, _ := w.(http.Flusher)
	flush := func() {
		if flusher != nil {
			flusher.Flush()
		}
	}

	if signals.Endpoint == "" || signals.Token == "" {
		writeLoginError(w, flush, cfg, signals.Endpoint, signals.ReturnURL,
			"Both MASS URL and token are required.")
		return
	}
	if _, err := url.Parse(signals.Endpoint); err != nil {
		writeLoginError(w, flush, cfg, signals.Endpoint, signals.ReturnURL,
			"Invalid MASS URL: "+err.Error())
		return
	}

	// Validate against MASS using a temp client so we don't pollute the live
	// client until success.
	probe := &mass.Client{
		Source:     cfg.Client.Source,
		HTTPClient: cfg.Client.HTTPClient,
	}
	probe.SetEndpoint(signals.Endpoint)
	probe.SetToken(signals.Token)

	ctx, cancel := context.WithTimeout(r.Context(), cfg.ValidateTimeout)
	defer cancel()
	if err := probe.Validate(ctx); err != nil {
		writeLoginError(w, flush, cfg, signals.Endpoint, signals.ReturnURL,
			"Could not connect: "+err.Error())
		return
	}

	if err := cfg.Store.Set(signals.Endpoint, signals.Token); err != nil {
		writeLoginError(w, flush, cfg, signals.Endpoint, signals.ReturnURL,
			"Failed to save token: "+err.Error())
		return
	}
	cfg.Client.SetEndpoint(signals.Endpoint)
	cfg.Client.SetToken(signals.Token)

	// Datastar @location redirect.
	writeSSE(w, "datastar-execute-script",
		fmt.Sprintf(`{"script":"window.location.href=%q"}`, signals.ReturnURL))
	flush()
}

func writeLoginError(w http.ResponseWriter, flush func(), cfg LoginConfig, endpoint, ret, msg string) {
	page := RenderLoginPage(LoginPageData{
		AppTitle:    cfg.AppTitle,
		EndpointURL: endpoint,
		ReturnURL:   ret,
		ErrorMsg:    msg,
		Theme:       cfg.Theme,
	})
	// Datastar patch-elements with full body replacement.
	writeSSE(w, "datastar-patch-elements",
		"selector body\nmode replace\nelements "+sseEscape(extractBody(page)))
	flush()
}

// writeSSE writes a single SSE event of the given type with the given data.
// data may contain newlines; each line is prefixed with "data: ".
func writeSSE(w http.ResponseWriter, event, data string) {
	_, _ = io.WriteString(w, "event: "+event+"\n")
	for line := range strings.SplitSeq(data, "\n") {
		_, _ = io.WriteString(w, "data: "+line+"\n")
	}
	_, _ = io.WriteString(w, "\n")
}

// sseEscape collapses any embedded newlines so the value fits in a single
// SSE field. Datastar's elements field accepts HTML on a single line.
func sseEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", " "), "\n", " ")
}

// extractBody returns the inner body markup of a full HTML page produced by
// uikit.Layout, so the Datastar patch-elements event can target <body>.
func extractBody(fullPage string) string {
	const startTag = "<body"
	const endTag = "</body>"
	i := strings.Index(fullPage, startTag)
	if i < 0 {
		return fullPage
	}
	gt := strings.Index(fullPage[i:], ">")
	if gt < 0 {
		return fullPage
	}
	start := i + gt + 1
	j := strings.Index(fullPage[start:], endTag)
	if j < 0 {
		return fullPage[start:]
	}
	return fullPage[start : start+j]
}

// LoginPageData drives RenderLoginPage.
type LoginPageData struct {
	AppTitle    string
	EndpointURL string // pre-fills the MASS URL field
	ReturnURL   string // hidden field carried back on submit
	ErrorMsg    string // optional error banner
	Theme       Theme
}

// RenderLoginPage produces the full HTML for the login screen.
func RenderLoginPage(d LoginPageData) string {
	if d.Theme == "" {
		d.Theme = ThemeDark
	}
	if d.EndpointURL == "" {
		d.EndpointURL = "http://localhost:3455"
	}
	if d.ReturnURL == "" {
		d.ReturnURL = "/"
	}

	errBlock := ""
	if d.ErrorMsg != "" {
		errBlock = fmt.Sprintf(
			`<sl-alert variant="danger" open class="mb-4"><sl-icon slot="icon" name="exclamation-octagon"></sl-icon>%s</sl-alert>`,
			html.EscapeString(d.ErrorMsg))
	}

	body := fmt.Sprintf(`
<div class="min-h-screen flex items-center justify-center p-6">
  <div class="w-full max-w-md p-8 rounded-lg" style="background: var(--mass-bg-panel); border: 1px solid var(--mass-border);">
    <h1 class="text-xl mb-1" style="color: var(--mass-text);">Connect to MASS</h1>
    <p class="text-sm mb-6" style="color: var(--mass-text-muted);">%s needs a MASS endpoint and auth token.</p>
    %s
    <div data-signals="{mass_endpoint:'%s', mass_token:'', mass_return:'%s'}">
      <sl-input label="MASS URL" data-bind="mass_endpoint" autocomplete="off" class="mb-4"></sl-input>
      <sl-input label="Auth token" type="password" data-bind="mass_token" autocomplete="off" toggle-password class="mb-6"></sl-input>
      <sl-button variant="primary" class="w-full" data-on-click="@post('%s')">Sign in</sl-button>
    </div>
  </div>
</div>`,
		html.EscapeString(d.AppTitle),
		errBlock,
		html.EscapeString(d.EndpointURL),
		html.EscapeString(d.ReturnURL),
		LoginPath,
	)

	return uikit.Layout(d.AppTitle+" — Sign in", body, string(d.Theme))
}
