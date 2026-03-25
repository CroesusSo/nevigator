package tls

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

	stdtls "crypto/tls"
)

// generateSelfSignedCert creates a self-signed certificate and key for testing.
func generateSelfSignedCert(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Co"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(24 * time.Hour),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyBytes := x509.MarshalPKCS1PrivateKey(priv)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes})
	return certPEM, keyPEM
}

// TestNewCertManager_NilConfig returns error when config is nil.
func TestNewCertManager_NilConfig(t *testing.T) {
	cm, err := NewCertManager(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
	if cm != nil {
		t.Fatal("expected nil CertManager")
	}
}

// TestCertManager_Reload_InvalidCert tests that reload returns error for bad certificate.
func TestCertManager_Reload_InvalidCert(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	os.WriteFile(certPath, []byte("not a real cert"), 0600)
	os.WriteFile(keyPath, []byte("not a real key"), 0600)

	cfg := &Config{
		CertificateFile: certPath,
		KeyFile:         keyPath,
	}
	_, err := NewCertManager(cfg)
	if err == nil {
		t.Fatal("expected error for invalid cert")
	}
}

// TestCertManager_Reload_ValidFiles tests that reload succeeds with valid files.
func TestCertManager_Reload_ValidFiles(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	certPEM, keyPEM := generateSelfSignedCert(t)
	os.WriteFile(certPath, certPEM, 0600)
	os.WriteFile(keyPath, keyPEM, 0600)

	cfg := &Config{
		CertificateFile: certPath,
		KeyFile:         keyPath,
	}
	cm, err := NewCertManager(cfg)
	if err != nil {
		t.Fatalf("NewCertManager: %v", err)
	}

	// Verify GetCertificate returns something
	_, err = cm.GetCertificate(&stdtls.ClientHelloInfo{})
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
}

// TestCertManager_GetCertificate_NoCert tests GetCertificate returns error when no certificate loaded.
func TestCertManager_GetCertificate_NoCert(t *testing.T) {
	cfg := &Config{} // no cert files
	cm, err := NewCertManager(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = cm.GetCertificate(&stdtls.ClientHelloInfo{})
	if err == nil {
		t.Fatal("expected GetCertificate error when no certs")
	}
}

// TestCertManager_GetConfig_ClientAuthMapping tests that all ClientAuthType values map correctly.
func TestCertManager_GetConfig_ClientAuthMapping(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	certPEM, keyPEM := generateSelfSignedCert(t)
	os.WriteFile(certPath, certPEM, 0600)
	os.WriteFile(keyPath, keyPEM, 0600)

	tests := []struct {
		input    ClientAuthType
		expected stdtls.ClientAuthType
	}{
		{NoClientCert, stdtls.NoClientCert},
		{RequestClientCert, stdtls.RequestClientCert},
		{RequireAnyClientCert, stdtls.RequireAnyClientCert},
		{VerifyClientCertIfGiven, stdtls.VerifyClientCertIfGiven},
		{RequireAndVerifyClientCert, stdtls.RequireAndVerifyClientCert},
	}
	for _, tt := range tests {
		cfg := &Config{
			CertificateFile: certPath,
			KeyFile:         keyPath,
			ClientAuth:      tt.input,
		}
		cm, err := NewCertManager(cfg)
		if err != nil {
			t.Fatalf("NewCertManager for %v: %v", tt.input, err)
		}
		got := cm.GetConfig().ClientAuth
		if got != tt.expected {
			t.Errorf("ClientAuth %v: got %v, want %v", tt.input, got, tt.expected)
		}
	}
}

// TestCertManager_GetConfig_CopiesData tests that GetConfig returns a fresh tls.Config each call.
func TestCertManager_GetConfig_CopiesData(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	certPEM, keyPEM := generateSelfSignedCert(t)
	os.WriteFile(certPath, certPEM, 0600)
	os.WriteFile(keyPath, keyPEM, 0600)

	cfg := &Config{
		CertificateFile: certPath,
		KeyFile:         keyPath,
		MinVersion:      stdtls.VersionTLS12,
		MaxVersion:      stdtls.VersionTLS13,
	}
	cm, err := NewCertManager(cfg)
	if err != nil {
		t.Fatalf("NewCertManager: %v", err)
	}

	got1 := cm.GetConfig()
	got2 := cm.GetConfig()
	if got1 == got2 {
		t.Error("GetConfig should return a new config each call")
	}
	if got1.MinVersion != stdtls.VersionTLS12 || got1.MaxVersion != stdtls.VersionTLS13 {
		t.Errorf("config values not copied correctly: %v", got1)
	}
}

// TestCertManager_Refresh tests that Refresh forces a reload.
func TestCertManager_Refresh(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	certPEM, keyPEM := generateSelfSignedCert(t)
	os.WriteFile(certPath, certPEM, 0600)
	os.WriteFile(keyPath, keyPEM, 0600)

	cfg := &Config{
		CertificateFile: certPath,
		KeyFile:         keyPath,
	}
	cm, err := NewCertManager(cfg)
	if err != nil {
		t.Fatalf("NewCertManager: %v", err)
	}

	if err := cm.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
}
