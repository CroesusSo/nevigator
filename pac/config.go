package pac

import (
	"net"
	"time"
)

// TLSConfig holds TLS configuration for the proxy server.
type TLSConfig struct {
	// CertificateFile is the path to the server certificate PEM file.
	CertificateFile string `yaml:"certificate_file" json:"certificate_file"`
	// KeyFile is the path to the server private key PEM file.
	KeyFile string `yaml:"key_file" json:"key_file"`
	// CAFile is the path to the certificate authority PEM file for client verification.
	CAFile string `yaml:"ca_file" json:"ca_file"`
	// MinVersion specifies the minimum TLS version that is acceptable.
	MinVersion uint16 `yaml:"min_version" json:"min_version"`
	// MaxVersion specifies the maximum TLS version that is acceptable.
	MaxVersion uint16 `yaml:"max_version" json:"max_version"`
	// CipherSuites specifies the allowed cipher suites for TLS connections.
	CipherSuites []uint16 `yaml:"cipher_suites" json:"cipher_suites"`
	// PreferServerCipherSuites indicates whether the server's cipher suite preferences
	// should be used instead of the client's.
	PreferServerCipherSuites bool `yaml:"prefer_server_cipher_suites" json:"prefer_server_cipher_suites"`
	// SessionTicketsDisabled indicates whether session tickets should be disabled.
	SessionTicketsDisabled bool `yaml:"session_tickets_disabled" json:"session_tickets_disabled"`
	// ClientAuth specifies the server's policy for client authentication.
	ClientAuth string `yaml:"client_auth" json:"client_auth"`
	// ClientCAs is the certificate authority set used to verify client certificates.
	ClientCAs string `yaml:"client_cas" json:"client_cas"`
	// InsecureSkipVerify controls whether a client verifies the server's certificate chain
	// and host name. If true, TLS accepts any certificate presented by the server and
	// any host name in that certificate. In this mode, TLS is susceptible to man-in-the-middle
	// attacks. This should be used only for testing.
	InsecureSkipVerify bool `yaml:"insecure_skip_verify" json:"insecure_skip_verify"`
	// VerifyPeerCertificate if set, is called after normal certificate verification
	// from either a TLS client or server. It receives the raw ASN.1 certificates
	// provided by the peer and also any verified chains that normal processing found.
	// If the function returns a non-nil error, the handshake is aborted and that error
	// is sent as an alert.
	VerifyPeerCertificate func(rawCerts [][]byte, verifiedChains [][][]byte) error `yaml:"-" json:"-"`
	// Timeout specifies the time limit for TLS handshake.
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// ExternalProxy represents an upstream SOCKS5 proxy with optional authentication.
type ExternalProxy struct {
	ID       string `yaml:"id" json:"id"`
	Address  string `yaml:"address" json:"address"`
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
}

// Route represents a routing decision from PAC evaluation.
type Route struct {
	Strategy   string // "TUN", "SOCKS5", "DIRECT", "EXTERNAL", "DEFAULT", "TLS_TUN", "TLS_SOCKS5", "TLS_DIRECT"
	Address    string // required for SOCKS5 and TLS_SOCKS5, ignored otherwise
	ExternalID string // used when Strategy == "EXTERNAL"
}

// EvaluatorConfig holds configuration for the PAC Evaluator.
type EvaluatorConfig struct {
	PACPath          string        // Path to PAC file
	LocalIP          string        // Value returned by myIpAddress()
	DNSTimeout       time.Duration // Timeout for dnsResolve/isResolvable
	EvalTimeout      time.Duration // Max time for Evaluate()
	EnableDNSCache   bool          // Enable in-memory DNS cache
	CustomDNSServers []string      `yaml:"custom_dns_servers" json:"custom_dns_servers"` // Optional custom DNS servers
	FallbackRoute    Route         // Used when PAC fails
}

// RouterConfig holds configuration for the Router.
type RouterConfig struct {
	TUNDevice       string
	TUNIP           net.IP          // If nil, will be discovered from TUNDevice
	DialTimeout     time.Duration   // Timeout for establishing outbound connections
	ExternalProxies []ExternalProxy // Registry of upstream SOCKS5 proxies (injected, not from YAML directly)
}

// ProxyConfig holds the overall proxy server configuration.
type ProxyConfig struct {
	HTTPListen   string
	SOCKS5Listen string
	AdminListen  string
	PACPath      string
	TUNDevice    string
	TUNIP        net.IP // auto-discovered if empty

	// TLS configuration for HTTPS proxy support
	TLSConfig TLSConfig `yaml:"tls"`
}
