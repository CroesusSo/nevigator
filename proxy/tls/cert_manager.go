package tls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"sync"
)

// CertManager manages TLS certificates for the proxy server.
type CertManager struct {
	mu           sync.RWMutex
	config       *Config
	certificates []tls.Certificate
	rootCAs      *x509.CertPool
	clientCAs    *x509.CertPool
}

// NewCertManager creates a new certificate manager.
func NewCertManager(config *Config) (*CertManager, error) {
	if config == nil {
		return nil, errors.New("TLS config is required")
	}

	cm := &CertManager{
		config: config,
	}

	if err := cm.reload(); err != nil {
		return nil, fmt.Errorf("failed to initialize certificate manager: %w", err)
	}

	return cm, nil
}

// reload loads or reloads certificates from disk.
func (cm *CertManager) reload() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var err error

	// Load server certificates
	if cm.config.CertificateFile != "" && cm.config.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cm.config.CertificateFile, cm.config.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to load server certificate: %w", err)
		}
		cm.certificates = []tls.Certificate{cert}
	} else {
		cm.certificates = nil
	}

	// Load root CAs for client verification
	if cm.config.CAFile != "" {
		cm.rootCAs, err = loadCAFile(cm.config.CAFile)
		if err != nil {
			return fmt.Errorf("failed to load CA file: %w", err)
		}
	} else {
		// Use system root CAs
		cm.rootCAs, err = x509.SystemCertPool()
		if err != nil {
			cm.rootCAs = x509.NewCertPool()
		}
	}

	// Load client CAs if specified
	if cm.config.ClientCAs != "" {
		cm.clientCAs, err = loadCAFile(cm.config.ClientCAs)
		if err != nil {
			return fmt.Errorf("failed to load client CA file: %w", err)
		}
	} else {
		cm.clientCAs = nil
	}

	return nil
}

// GetCertificate returns a certificate for TLS handshake.
func (cm *CertManager) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if len(cm.certificates) == 0 {
		return nil, errors.New("no certificate configured")
	}
	return &cm.certificates[0], nil
}

// GetConfig returns a TLS configuration based on the current state.
func (cm *CertManager) GetConfig() *tls.Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	config := &tls.Config{
		Certificates:             cm.certificates,
		RootCAs:                  cm.rootCAs,
		ClientCAs:                cm.clientCAs,
		MinVersion:               cm.config.MinVersion,
		MaxVersion:               cm.config.MaxVersion,
		CipherSuites:             cm.config.CipherSuites,
		PreferServerCipherSuites: cm.config.PreferServerCipherSuites,
		SessionTicketsDisabled:   cm.config.SessionTicketsDisabled,
		InsecureSkipVerify:       cm.config.InsecureSkipVerify,
	}

	// Set client authentication based on config
	switch cm.config.ClientAuth {
	case NoClientCert:
		config.ClientAuth = tls.NoClientCert
	case RequestClientCert:
		config.ClientAuth = tls.RequestClientCert
	case RequireAnyClientCert:
		config.ClientAuth = tls.RequireAnyClientCert
	case VerifyClientCertIfGiven:
		config.ClientAuth = tls.VerifyClientCertIfGiven
	case RequireAndVerifyClientCert:
		config.ClientAuth = tls.RequireAndVerifyClientCert
	}

	// Set custom verification callback if provided
	if cm.config.VerifyPeerCertificate != nil {
		config.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			// Convert [][]*x509.Certificate to [][][]byte as expected by our config type
			var chains [][][]byte
			for _, chain := range verifiedChains {
				var bchain [][]byte
				for _, cert := range chain {
					bchain = append(bchain, cert.Raw)
				}
				chains = append(chains, bchain)
			}
			return cm.config.VerifyPeerCertificate(rawCerts, chains)
		}
	}

	return config
}

// loadCAFile loads a PEM-encoded CA file and returns a cert pool.
func loadCAFile(path string) (*x509.CertPool, error) {
	pemData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(pemData); !ok {
		return nil, fmt.Errorf("failed to parse CA file %s", path)
	}

	return certPool, nil
}

// Refresh forces a reload of certificates from disk.
func (cm *CertManager) Refresh() error {
	return cm.reload()
}
