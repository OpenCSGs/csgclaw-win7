package httptrust

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func fixtureCertificate(t *testing.T, template, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if parent == nil {
		parent, parentKey = template, key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key
}

func TestPrivateRootsAndCertificateValidation(t *testing.T) {
	now := time.Now()
	root, key := fixtureCertificate(t, &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "private fixture CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}, nil, nil)
	leaf, _ := fixtureCertificate(t, &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{"private.example"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Minute), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}, root, key)
	pool, err := rootPool([]*x509.Certificate{root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	options := x509.VerifyOptions{DNSName: "private.example", Roots: pool, CurrentTime: now}
	if _, err := leaf.Verify(options); err != nil {
		t.Fatalf("system private CA was lost: %v", err)
	}
	options.DNSName = "other.example"
	if _, err := leaf.Verify(options); err == nil {
		t.Fatal("wrong hostname was accepted")
	}
	options.DNSName, options.CurrentTime = "private.example", now.Add(2*time.Minute)
	if _, err := leaf.Verify(options); err == nil {
		t.Fatal("expired certificate was accepted")
	}
	untrusted, err := rootPool(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	options.Roots, options.CurrentTime = untrusted, now
	if _, err := leaf.Verify(options); err == nil {
		t.Fatal("unknown authority was accepted")
	}
	blocked := blockedCertificates{certificateID(root): true}
	pool, err = rootPool([]*x509.Certificate{root}, blocked)
	if err != nil {
		t.Fatal(err)
	}
	options.Roots = pool
	if _, err := leaf.Verify(options); err == nil {
		t.Fatal("explicitly distrusted root was accepted")
	}
	config := configureTLS(nil, untrusted, blockedCertificates{certificateID(leaf): true})
	if config.InsecureSkipVerify {
		t.Fatal("certificate verification was disabled")
	}
	if err := config.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}}); err == nil {
		t.Fatal("explicitly distrusted leaf was accepted")
	}
}

func TestExistingTLSConfigurationIsPreserved(t *testing.T) {
	want := errors.New("existing certificate policy")
	base := &tls.Config{ServerName: "private.example", MinVersion: tls.VersionTLS13, VerifyConnection: func(tls.ConnectionState) error { return want }}
	pool, err := rootPool(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	config := configureTLS(base, pool, nil)
	if config == base || base.RootCAs != nil || config.MinVersion != base.MinVersion || config.ServerName != base.ServerName {
		t.Fatal("existing TLS configuration was changed")
	}
	if err := config.VerifyConnection(tls.ConnectionState{}); !errors.Is(err, want) {
		t.Fatal("existing certificate policy was discarded")
	}
}

func TestHTTPSAndWebsocketRequireATrustedCertificate(t *testing.T) {
	now := time.Now()
	root, key := fixtureCertificate(t, &x509.Certificate{SerialNumber: big.NewInt(10), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}, nil, nil)
	leaf, leafKey := fixtureCertificate(t, &x509.Certificate{SerialNumber: big.NewInt(11), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}, root, key)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ws" {
			conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
			if err == nil {
				conn.Close()
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{leaf.Raw}, PrivateKey: leafKey}}}
	server.StartTLS()
	t.Cleanup(server.Close)
	for _, trusted := range []bool{true, false} {
		var system []*x509.Certificate
		if trusted {
			system = []*x509.Certificate{root}
		}
		pool, err := rootPool(system, nil)
		if err != nil {
			t.Fatal(err)
		}
		config := configureTLS(nil, pool, nil)
		transport := &http.Transport{TLSClientConfig: config}
		response, err := (&http.Client{Transport: transport}).Get(server.URL)
		if (err == nil) != trusted {
			t.Fatalf("HTTPS trusted=%v: %v", trusted, err)
		}
		if response != nil {
			response.Body.Close()
		}
		transport.CloseIdleConnections()
		dialer := websocket.Dialer{TLSClientConfig: config, HandshakeTimeout: 5 * time.Second}
		conn, response, err := dialer.Dial("wss"+strings.TrimPrefix(server.URL, "https")+"/ws", nil)
		if (err == nil) != trusted {
			t.Fatalf("WebSocket trusted=%v: %v", trusted, err)
		}
		if response != nil {
			response.Body.Close()
		}
		if conn != nil {
			conn.Close()
		}
	}
}
