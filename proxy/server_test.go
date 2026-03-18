package proxy

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/pac"
)

// generateSelfSignedCert creates a temporary self-signed certificate and key.
func generateSelfSignedCert(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	// Generate RSA private key
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}
	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(24 * time.Hour),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
	}
	template.DNSNames = []string{"localhost"}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}
	// Write cert to file
	tmpDir := t.TempDir()
	certPath = filepath.Join(tmpDir, "cert.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0600); err != nil {
		t.Fatalf("failed to write cert: %v", err)
	}
	// Write private key to file
	keyPath = filepath.Join(tmpDir, "key.pem")
	privDER := x509.MarshalPKCS1PrivateKey(priv)
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privDER}), 0600); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}
	return certPath, keyPath
}

// TestNewServer_HTTPServerCreatedWhenConfigured tests that an HTTPS server is created
// when HTTPSListen is set and TLSConfig is valid.
func TestNewServer_HTTPServerCreatedWhenConfigured(t *testing.T) {
	certPath, keyPath := generateSelfSignedCert(t)
	cfg := pac.ProxyConfig{
		HTTPSListen: "127.0.0.1:0", // port 0 = random free port
		TLSConfig: pac.TLSConfig{
			CertificateFile: certPath,
			KeyFile:         keyPath,
		},
	}
	s := NewServer(cfg, nil, nil)
	if s.httpsServer == nil {
		t.Fatal("expected httpsServer to be non-nil when HTTPSListen and TLSConfig are valid")
	}
}

// TestNewServer_NoHTTPServerWhenNotConfigured tests that httpsServer is nil when HTTPSListen is empty.
func TestNewServer_NoHTTPServerWhenNotConfigured(t *testing.T) {
	cfg := pac.ProxyConfig{
		HTTPSListen: "", // empty
	}
	s := NewServer(cfg, nil, nil)
	if s.httpsServer != nil {
		t.Fatal("expected httpsServer to be nil when HTTPSListen is empty")
	}
}

// TestNewServer_HTTPServerNotCreatedWhenTLSInvalid tests that httpsServer is nil when TLS config is invalid.
func TestNewServer_HTTPServerNotCreatedWhenTLSInvalid(t *testing.T) {
	cfg := pac.ProxyConfig{
		HTTPSListen: "127.0.0.1:0",
		TLSConfig: pac.TLSConfig{
			CertificateFile: "/nonexistent/cert.pem",
			KeyFile:         "/nonexistent/key.pem",
		},
	}
	// Expect a warning log, but httpsServer should be nil
	s := NewServer(cfg, nil, nil)
	if s.httpsServer != nil {
		t.Fatal("expected httpsServer to be nil when TLSConfig is invalid")
	}
}
