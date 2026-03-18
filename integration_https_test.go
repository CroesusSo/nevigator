package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/proxy"
	"github.com/yourorg/proxy-pac/router"
)

// generateSelfSignedCert creates a temporary self-signed certificate for testing.
func generateSelfSignedCert(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}
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
	template.DNSNames = []string{"localhost", "127.0.0.1"}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}
	tmpDir := t.TempDir()
	certPath = filepath.Join(tmpDir, "cert.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0600); err != nil {
		t.Fatalf("failed to write cert: %v", err)
	}
	keyPath = filepath.Join(tmpDir, "key.pem")
	privDER := x509.MarshalPKCS1PrivateKey(priv)
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privDER}), 0600); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}
	return certPath, keyPath
}

// TestHTTPSProxy_ListenerStartup tests that HTTPS proxy listener starts and accepts TLS connections.
func TestHTTPSProxy_ListenerStartup(t *testing.T) {
	// Generate self-signed cert
	certPath, keyPath := generateSelfSignedCert(t)

	// Create a minimal PAC file
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "test.pac")
	pacContent := `function FindProxyForURL(url, host) { return "DIRECT"; }`
	if err := os.WriteFile(pacPath, []byte(pacContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Configure proxy with HTTPS on a specific port (use high number to avoid conflicts)
	httpsPort := "127.0.0.1:18443"
	proxyCfg := pac.ProxyConfig{
		HTTPListen:  "127.0.0.1:0",
		HTTPSListen: httpsPort,
		AdminListen: "127.0.0.1:0",
		PACPath:     pacPath,
		TLSConfig: pac.TLSConfig{
			CertificateFile: certPath,
			KeyFile:         keyPath,
		},
	}

	evalCfg := pac.EvaluatorConfig{
		PACPath:        pacPath,
		LocalIP:        "127.0.0.1",
		DNSTimeout:     50 * time.Millisecond,
		EvalTimeout:    50 * time.Millisecond,
		EnableDNSCache: false,
		FallbackRoute:  pac.Route{Strategy: "DIRECT"},
	}
	evaluator, err := pac.NewEvaluator(evalCfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer evaluator.Close()

	routerCfg := pac.RouterConfig{
		TUNIP:       nil,
		DialTimeout: 5 * time.Second,
	}
	r, err := router.NewRouter(routerCfg)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	serv := proxy.NewServer(proxyCfg, evaluator, r)
	if err := serv.Start(); err != nil {
		t.Fatalf("Server Start: %v", err)
	}
	defer serv.Stop()

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	// Verify Status reports the HTTPS address
	status := serv.Status()
	if status.HTTPSListen != httpsPort {
		t.Errorf("HTTPSListen in status = %q, want %q", status.HTTPSListen, httpsPort)
	}

	// Try to establish a TLS connection to the HTTPS proxy
	conn, err := tls.Dial("tcp", httpsPort, &tls.Config{
		InsecureSkipVerify: true, // skip cert verification for test
		ServerName:         "localhost",
	})
	if err != nil {
		t.Fatalf("TLS connection to HTTPS proxy failed: %v", err)
	}
	conn.Close()
}

// TestHTTPSProxy_DisabledWhenNoTLS tests that HTTPS listener is not started when TLS config is invalid.
func TestHTTPSProxy_DisabledWhenNoTLS(t *testing.T) {
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "test.pac")
	if err := os.WriteFile(pacPath, []byte(`function FindProxyForURL(url, host) { return "DIRECT"; }`), 0644); err != nil {
		t.Fatal(err)
	}

	httpsPort := "127.0.0.1:18444" // different port
	// Configure HTTPSListen but NO TLS certificate
	proxyCfg := pac.ProxyConfig{
		HTTPSListen: httpsPort,
		TLSConfig:   pac.TLSConfig{}, // empty
	}

	evalCfg := pac.EvaluatorConfig{
		PACPath:       pacPath,
		LocalIP:       "127.0.0.1",
		FallbackRoute: pac.Route{Strategy: "DIRECT"},
	}
	evaluator, _ := pac.NewEvaluator(evalCfg)
	defer evaluator.Close()

	routerCfg := pac.RouterConfig{TUNIP: nil}
	r, _ := router.NewRouter(routerCfg)

	serv := proxy.NewServer(proxyCfg, evaluator, r)
	if err := serv.Start(); err != nil {
		t.Fatalf("Server Start: %v", err)
	}
	defer serv.Stop()

	time.Sleep(200 * time.Millisecond)

	// HTTPS should not be listening; connection should be refused
	_, err := net.Dial("tcp", httpsPort)
	if err == nil {
		t.Fatal("expected connection to fail when TLS not configured, but succeeded")
	}
}
