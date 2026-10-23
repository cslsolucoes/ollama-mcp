package ollama

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tlsServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"models":[{"name":"qwen3-ia"}]}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func writeCA(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ca.pem")
	b := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// otherCertServer is a TLS server whose certificate is NOT the httptest one
// (all httptest TLS servers share a single built-in certificate).
func otherCertServer(t *testing.T) *httptest.Server {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(42), Subject: pkix.Name{CommonName: "other"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "{}") }))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func tlsClient(t *testing.T, srv *httptest.Server, ca string, insecure bool) (*Client, error) {
	t.Helper()
	c, err := New(Options{BaseURL: srv.URL, APIKey: token, TLSCA: ca, Insecure: insecure, Timeout: 5 * time.Second})
	if c != nil {
		t.Cleanup(c.Close)
	}
	return c, err
}

// TEST-005, scenario 1: an untrusted certificate is rejected and the message
// points at --tls-ca.
func TestTLSUntrustedRejected(t *testing.T) {
	srv := tlsServer(t)
	c, err := tlsClient(t, srv, "", false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Tags(context.Background())
	e := wantKind(t, err, ErrTLS)
	if !strings.Contains(e.Msg, "--tls-ca") {
		t.Fatalf("msg: %q", e.Msg)
	}
}

// Scenario 2: --insecure accepts it.
func TestTLSInsecureAccepts(t *testing.T) {
	srv := tlsServer(t)
	c, err := tlsClient(t, srv, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Tags(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// Scenario 3: a pinned CA is accepted (system pool + PEM).
func TestTLSPinnedCAAccepts(t *testing.T) {
	srv := tlsServer(t)
	c, err := tlsClient(t, srv, writeCA(t, srv), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Tags(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// Scenario 4: pinning one CA does not turn verification off for another cert.
func TestTLSPinnedCADoesNotDisableVerification(t *testing.T) {
	good, other := tlsServer(t), otherCertServer(t)
	c, err := tlsClient(t, other, writeCA(t, good), false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Tags(context.Background())
	wantKind(t, err, ErrTLS)
}

// Scenario 5: a missing CA file or a PEM without certificate is an error.
func TestTLSBadCAFile(t *testing.T) {
	srv := tlsServer(t)
	if _, err := tlsClient(t, srv, filepath.Join(t.TempDir(), "missing.pem"), false); err == nil || !strings.Contains(err.Error(), "--tls-ca") {
		t.Fatalf("missing file: %v", err)
	}
	empty := filepath.Join(t.TempDir(), "empty.pem")
	os.WriteFile(empty, []byte("-----BEGIN NOTHING-----\nxx\n-----END NOTHING-----\n"), 0o600)
	if _, err := tlsClient(t, srv, empty, false); err == nil || !strings.Contains(err.Error(), "nenhum certificado PEM") {
		t.Fatalf("pem without certificate: %v", err)
	}
}

func TestTLSInsecureIgnoresCA(t *testing.T) {
	srv := tlsServer(t)
	// with --insecure the CA is not even read, so a bad path is not an error
	if _, err := tlsClient(t, srv, filepath.Join(t.TempDir(), "missing.pem"), true); err != nil {
		t.Fatal(err)
	}
}

func TestTLSStreamOverTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeStream(w, genStream) }))
	t.Cleanup(srv.Close)
	c, err := tlsClient(t, srv, writeCA(t, srv), false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Generate(context.Background(), GenerateRequest{Model: "qwen3-ia", Prompt: "x"})
	if err != nil || got.Response != "Olá, mundo" {
		t.Fatalf("%+v %v", got, err)
	}
}
