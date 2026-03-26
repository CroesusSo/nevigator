package pac

import (
	"encoding/json"
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

// KeepaliveConfig holds keepalive settings for SSH tunnels.
type KeepaliveConfig struct {
	Enabled  bool          `yaml:"enabled" json:"enabled"`   // Whether keepalive is enabled
	Interval time.Duration `yaml:"interval" json:"interval"` // How often to send keepalive probes (e.g., "30s")
}

// UnmarshalYAML implements YAML unmarshaling for KeepaliveConfig with defaults.
func (c *KeepaliveConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Define a temporary alias to avoid infinite recursion
	type plain KeepaliveConfig
	aux := &struct {
		*plain
	}{
		plain: (*plain)(c),
	}
	if err := unmarshal(aux); err != nil {
		return err
	}
	// Apply defaults: if Enabled is true and Interval is zero, set to 30s
	if c.Enabled && c.Interval == 0 {
		c.Interval = 30 * time.Second
	}
	return nil
}

// UnmarshalJSON implements JSON unmarshaling for KeepaliveConfig with defaults.
func (c *KeepaliveConfig) UnmarshalJSON(data []byte) error {
	// Define a temporary alias to avoid infinite recursion
	type plain KeepaliveConfig
	aux := &struct {
		*plain
	}{
		plain: (*plain)(c),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	// Apply defaults: if Enabled is true and Interval is zero, set to 30s
	if c.Enabled && c.Interval == 0 {
		c.Interval = 30 * time.Second
	}
	return nil
}

// ExternalProxy represents an upstream proxy (SOCKS5 or SSH) with optional authentication.
type ExternalProxy struct {
	ID       string `yaml:"id" json:"id"`
	Kind     string `yaml:"kind,omitempty" json:"kind,omitempty"` // "socks5" (default) or "ssh"
	Address  string `yaml:"address" json:"address"`               // SOCKS5 host:port; for SSH: host[:port] or user@host[:port]
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`

	// SSH-only fields (ignored if Kind != "ssh")
	SSHPort            int             `yaml:"ssh_port,omitempty" json:"ssh_port,omitempty"`       // default 22; overrides any port in Address
	AuthMethod         string          `yaml:"auth_method,omitempty" json:"auth_method,omitempty"` // "password" or "key" (required if Kind=="ssh")
	PrivateKeyPath     string          `yaml:"private_key_path,omitempty" json:"private_key_path,omitempty"`
	HostKeyCheck       string          `yaml:"host_key_check,omitempty" json:"host_key_check,omitempty"`             // "strict" (default) or "ignore"
	HostKeyFingerprint string          `yaml:"host_key_fingerprint,omitempty" json:"host_key_fingerprint,omitempty"` // optional hex fingerprint for pinning
	Keepalive          KeepaliveConfig `yaml:"keepalive,omitempty" json:"keepalive,omitempty"`                       // Optional keepalive configuration
}

// Route represents a routing decision from PAC evaluation.
type Route struct {
	Strategy   string // "SOCKS5", "DIRECT", "EXTERNAL", "DEFAULT"
	Address    string // required for SOCKS5, ignored otherwise
	ExternalID string // used when Strategy == "EXTERNAL"
}

// EvaluatorConfig holds configuration for the PAC Evaluator.
type EvaluatorConfig struct {
	PACPath            string        // Path to PAC file
	LocalIP            string        // Value returned by myIpAddress()
	DNSTimeout         time.Duration // Timeout for dnsResolve/isResolvable
	EvalTimeout        time.Duration // Max time for Evaluate()
	EnableDNSCache     bool          // Enable in-memory DNS cache
	DNSMaxEntries      int           `yaml:"dns_max_entries" json:"dns_max_entries"`       // Max DNS cache entries (0 = unlimited)
	DNSTTL             time.Duration `yaml:"dns_ttl" json:"dnsttl"`                        // TTL for DNS cache entries
	CustomDNSServers   []string      `yaml:"custom_dns_servers" json:"custom_dns_servers"` // Optional custom DNS servers
	FallbackRoute      Route         // Used when PAC fails
	EnableURLGlob      bool          `yaml:"enable_url_glob" json:"enable_url_glob"`           // Enable URL-glob rule generation
	EnableDeepestMatch bool          `yaml:"enable_deepest_match" json:"enable_deepest_match"` // Use deepest/specific match semantics

	// Cache configuration for native rule engine
	CacheEnabled bool          `yaml:"cache_enabled" json:"cache_enabled"`
	CacheMaxSize int           `yaml:"cache_max_size" json:"cache_max_size"`
	CacheTTL     time.Duration `yaml:"cache_ttl" json:"cache_ttl"`
}

// RouterConfig holds configuration for the Router.
type RouterConfig struct {
	DialTimeout     time.Duration   // Timeout for establishing outbound connections
	ExternalProxies []ExternalProxy // Registry of upstream SOCKS5 proxies (injected, not from YAML directly)
}

// ProxyConfig holds the overall proxy server configuration.
type ProxyConfig struct {
	HTTPListen   string
	SOCKS5Listen string
	AdminListen  string
	HTTPSListen  string // Optional TLS-wrapped HTTP proxy
	PACPath      string

	// TLS configuration for HTTPS proxy support
	TLSConfig TLSConfig `yaml:"tls"`
}
