package masstls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientConfig_SystemRoots(t *testing.T) {
	cfg, err := ClientConfig("")
	require.NoError(t, err)
	require.Nil(t, cfg.RootCAs, "empty caFile uses the system pool")
	require.Equal(t, uint16(0x0303), cfg.MinVersion) // TLS 1.2
}

func TestClientConfig_CustomCA(t *testing.T) {
	pemPath := writeSelfSignedCA(t)
	cfg, err := ClientConfig(pemPath)
	require.NoError(t, err)
	require.NotNil(t, cfg.RootCAs, "a custom CA is loaded into the pool")
}

func TestClientConfig_Errors(t *testing.T) {
	_, err := ClientConfig(filepath.Join(t.TempDir(), "missing.pem"))
	require.Error(t, err, "missing file errors")

	notPEM := filepath.Join(t.TempDir(), "junk.pem")
	require.NoError(t, os.WriteFile(notPEM, []byte("not a certificate"), 0o600))
	_, err = ClientConfig(notPEM)
	require.Error(t, err, "a file with no valid certificates errors")
}

func TestHTTPClient(t *testing.T) {
	hc, err := HTTPClient("")
	require.NoError(t, err)
	tr, ok := hc.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, tr.TLSClientConfig)

	pemPath := writeSelfSignedCA(t)
	hc, err = HTTPClient(pemPath)
	require.NoError(t, err)
	tr = hc.Transport.(*http.Transport)
	require.NotNil(t, tr.TLSClientConfig.RootCAs)
}

// The client bounds connection setup, but must never bound the response wait:
// Grimoire embeds through it against a cold GGUF and long-polls the PDF
// converter's job result, both for minutes, each under its own context deadline.
// A total or response-header timeout here would break them.
func TestHTTPClient_TimeoutContract(t *testing.T) {
	hc, err := HTTPClient("")
	require.NoError(t, err)
	require.Zero(t, hc.Timeout, "a total timeout would cut off legitimate long gateway calls")

	tr, ok := hc.Transport.(*http.Transport)
	require.True(t, ok)
	require.Zero(t, tr.ResponseHeaderTimeout, "the gateway may legitimately take minutes to answer")
	require.Equal(t, tlsHandshakeTimeout, tr.TLSHandshakeTimeout, "TLS handshake is bounded")
	require.NotNil(t, tr.DialContext, "dialing is bounded by our own dialer")
	// Cloned from DefaultTransport, so environment proxies and HTTP/2 still work.
	require.NotNil(t, tr.Proxy)
	require.True(t, tr.ForceAttemptHTTP2)
}

// writeSelfSignedCA generates a throwaway self-signed CA certificate, writes it
// as PEM to a temp file, and returns the path.
func writeSelfSignedCA(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Unix(0, 0),
		NotAfter:              time.Unix(1<<31-1, 0),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	return path
}
