// Package masstls builds client TLS configuration for talking to a MASS
// gateway over HTTPS, including support for a private or self-signed CA.
package masstls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

// Connection-setup budget for the returned client. Reaching a gateway and
// completing its TLS handshake is fast or broken — there is no legitimate slow
// case — so these are stated here rather than inherited from the stdlib
// defaults (which allow 30s to dial).
//
// Deliberately absent: ResponseHeaderTimeout and a total Client.Timeout. Once
// the request is on the wire, only the caller knows how long an answer may
// legitimately take, and for this client the answer is "minutes": Grimoire
// embeds through it against a cold GGUF that takes minutes to load, and its PDF
// converter long-polls GET /.v1/Jobs/{id}?wait=1 for a page's result. Both bound
// their own calls with a context deadline. Capping the response wait here would
// break them; cancellation stays the caller's job.
const (
	dialTimeout         = 10 * time.Second
	tlsHandshakeTimeout = 10 * time.Second
	dialKeepAlive       = 30 * time.Second
)

// ClientConfig returns a *tls.Config for connecting to a MASS gateway. When
// caFile is non-empty the CA it contains (PEM) is added to the trust pool, so a
// private-CA or self-signed gateway is accepted; when empty the system root
// pool is used.
func ClientConfig(caFile string) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile == "" {
		return cfg, nil
	}
	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("CA file %q contains no valid certificates", caFile)
	}
	cfg.RootCAs = pool
	return cfg, nil
}

// HTTPClient returns an *http.Client whose transport trusts the given CA (or the
// system roots when caFile is empty). Passing an empty caFile yields a client
// equivalent to the default, so callers can always route through this with the
// stored CA path, configured or not.
//
// The transport bounds connection setup only — see the timeout constants for why
// the response wait is left to the caller's context. It is cloned from
// [http.DefaultTransport] so proxy support, HTTP/2 and connection pooling behave
// as they do everywhere else.
func HTTPClient(caFile string) (*http.Client, error) {
	cfg, err := ClientConfig(caFile)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = cfg
	transport.DialContext = (&net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: dialKeepAlive,
	}).DialContext
	transport.TLSHandshakeTimeout = tlsHandshakeTimeout
	return &http.Client{Transport: transport}, nil
}
