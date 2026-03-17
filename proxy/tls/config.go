package tls

import (
	"time"
)

// Config holds TLS configuration for the proxy server.
type Config struct {
	// CertificateFile is the path to the server certificate PEM file.
	CertificateFile string
	// KeyFile is the path to the server private key PEM file.
	KeyFile string
	// CAFile is the path to the certificate authority PEM file for client verification.
	CAFile string
	// MinVersion specifies the minimum TLS version that is acceptable.
	MinVersion uint16
	// MaxVersion specifies the maximum TLS version that is acceptable.
	MaxVersion uint16
	// CipherSuites specifies the allowed cipher suites for TLS connections.
	CipherSuites []uint16
	// PreferServerCipherSuites indicates whether the server's cipher suite preferences
	// should be used instead of the client's.
	PreferServerCipherSuites bool
	// SessionTicketsDisabled indicates whether session tickets should be disabled.
	SessionTicketsDisabled bool
	// ClientAuth specifies the server's policy for client authentication.
	ClientAuth ClientAuthType
	// ClientCAs is the certificate authority set used to verify client certificates.
	ClientCAs string
	// InsecureSkipVerify controls whether a client verifies the server's certificate chain
	// and host name. If true, TLS accepts any certificate presented by the server and
	// any host name in that certificate. In this mode, TLS is susceptible to man-in-the-middle
	// attacks. This should be used only for testing.
	InsecureSkipVerify bool
	// VerifyPeerCertificate if set, is called after normal certificate verification
	// from either a TLS client or server. It receives the raw ASN.1 certificates
	// provided by the peer and also any verified chains that normal processing found.
	// If the function returns a non-nil error, the handshake is aborted and that error
	// is sent as an alert.
	VerifyPeerCertificate func(rawCerts [][]byte, verifiedChains [][][]byte) error
	// Timeout specifies the time limit for TLS handshake.
	Timeout time.Duration
}

// ClientAuthType represents the client authentication policy.
type ClientAuthType int

const (
	// NoClientCert requests no client certificate.
	NoClientCert ClientAuthType = iota
	// RequestClientCert requests a client certificate but does not require it.
	RequestClientCert
	// RequireAnyClientCert requires a client certificate but does not verify it.
	RequireAnyClientCert
	// VerifyClientCertIfGiven requests a client certificate and verifies it if provided.
	VerifyClientCertIfGiven
	// RequireAndVerifyClientCert requires a client certificate and verifies it.
	RequireAndVerifyClientCert
)
