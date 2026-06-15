package main

import (
	"bufio"
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
	"os"
	"path/filepath"
	"time"
)

// devTLSConfig returns a TLS config backed by a self-signed certificate for
// localhost. It exists so the dev server can speak HTTP/2: browsers only
// negotiate HTTP/2 over TLS, and HTTP/2 multiplexes every SSE stream and
// request over a single connection — sidestepping the HTTP/1.1
// ~6-connections-per-host limit that otherwise starves htmx requests (e.g. the
// SSO logout POST) once the gallery page opens one SSE per card.
//
// The certificate is cached on disk (see devCertDir) and reused across
// restarts, so the browser only has to trust the self-signed cert once rather
// than on every air reload. It is a development convenience, not a trust
// anchor.
func devTLSConfig() (*tls.Config, error) {
	cert, err := loadOrCreateDevCert()
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		// Advertise HTTP/2 first; net/http wires in the h2 handler for TLS
		// servers whose NextProtos include "h2". This is honoured by
		// (*http.Server).Serve too, not just ListenAndServeTLS.
		NextProtos: []string{"h2", "http/1.1"},
		MinVersion: tls.VersionTLS12,
	}, nil
}

// devCertDir is where the cached dev certificate lives. Override with
// GOHTMXELM_TLS_DIR; defaults to ./.devtls (gitignored).
func devCertDir() string {
	if d := os.Getenv("GOHTMXELM_TLS_DIR"); d != "" {
		return d
	}
	return ".devtls"
}

// loadOrCreateDevCert reuses a cached, still-valid certificate when present,
// otherwise generates a fresh one and persists it. A stable cert means the
// browser's "proceed anyway" exception survives air reloads.
func loadOrCreateDevCert() (tls.Certificate, error) {
	dir := devCertDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	if cert, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
		if leaf, perr := x509.ParseCertificate(cert.Certificate[0]); perr == nil {
			// Keep a day of headroom so a near-expiry cert is rotated early.
			if time.Now().Before(leaf.NotAfter.Add(-24 * time.Hour)) {
				return cert, nil
			}
		}
	}

	cert, certPEM, keyPEM, err := generateDevCert()
	if err != nil {
		return tls.Certificate{}, err
	}
	if mkErr := os.MkdirAll(dir, 0o755); mkErr == nil {
		_ = os.WriteFile(certPath, certPEM, 0o644)
		_ = os.WriteFile(keyPath, keyPEM, 0o600)
	}
	return cert, nil
}

// generateDevCert mints a fresh self-signed localhost certificate and returns
// both the usable tls.Certificate and its PEM encodings for caching.
func generateDevCert() (tls.Certificate, []byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, nil, fmt.Errorf("generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, nil, nil, fmt.Errorf("generate serial: %w", err)
	}

	now := time.Now()
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{Organization: []string{"gohtmxelm dev"}, CommonName: "localhost"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, nil, nil, fmt.Errorf("create certificate: %w", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return tls.Certificate{}, nil, nil, fmt.Errorf("marshal key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	return cert, certPEM, keyPEM, nil
}

// serveTLSWithRedirect serves the TLS server but also answers plain-HTTP
// requests that land on the HTTPS port with a redirect to https, instead of
// letting them fail as noisy "client sent an HTTP request to an HTTPS server"
// handshake errors. It sniffs the first byte of each connection: a TLS
// ClientHello always begins with the handshake record type 0x16, so anything
// else is treated as plaintext HTTP.
func serveTLSWithRedirect(server *http.Server) error {
	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}
	sniff := &redirectListener{Listener: ln}
	// Wrapping the sniffing listener in tls.NewListener TLS-wraps only the
	// connections it returns (the real TLS ones); plaintext connections are
	// handled and closed inside Accept and never reach the TLS layer.
	return server.Serve(tls.NewListener(sniff, server.TLSConfig))
}

// redirectListener only ever returns genuine TLS connections; plaintext HTTP
// connections are served a redirect-to-https response out of band.
type redirectListener struct {
	net.Listener
}

func (l *redirectListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		first := make([]byte, 1)
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		n, rerr := conn.Read(first)
		_ = conn.SetReadDeadline(time.Time{})
		if rerr != nil || n == 0 {
			_ = conn.Close()
			continue
		}
		pc := &prefixConn{Conn: conn, prefix: first[:n]}
		if first[0] == 0x16 { // TLS handshake record → real HTTPS client
			return pc, nil
		}
		go redirectToHTTPS(pc) // plaintext HTTP → bounce to https
	}
}

// prefixConn replays bytes already peeked off the wire before reading more, so
// the sniffed first byte is not lost to the TLS handshake or the HTTP parser.
type prefixConn struct {
	net.Conn
	prefix []byte
}

func (c *prefixConn) Read(p []byte) (int, error) {
	if len(c.prefix) > 0 {
		n := copy(p, c.prefix)
		c.prefix = c.prefix[n:]
		return n, nil
	}
	return c.Conn.Read(p)
}

func redirectToHTTPS(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	req, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return
	}
	host := req.Host
	if host == "" {
		host = "localhost"
	}
	target := "https://" + host + req.URL.RequestURI()
	body := "redirecting to " + target + "\n"
	// 302 (not 308) so browsers don't cache the redirect — important for dev,
	// where the same port may later be served over plain HTTP via TLS=0.
	fmt.Fprintf(conn, "HTTP/1.1 302 Found\r\n"+
		"Location: %s\r\n"+
		"Content-Type: text/plain; charset=utf-8\r\n"+
		"Content-Length: %d\r\n"+
		"Connection: close\r\n\r\n%s", target, len(body), body)
}
