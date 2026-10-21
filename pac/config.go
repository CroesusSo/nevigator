package pac

import (
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
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

	// TCP tunes the underlying socket's keepalive probes. This complements
	// Enabled/Interval: the SSH request proves end-to-end liveness, while TCP
	// probes detect a dead transport. Independent of Enabled. A pointer keeps it
	// out of the admin API payload unless explicitly configured.
	TCP *TCPKeepAliveConfig `yaml:"tcp,omitempty" json:"tcp,omitempty"`
}

func (k *KeepaliveConfig) UnmarshalYAML(value *yaml.Node) error {
	// 1. Define an alias to avoid an infinite loop during Unmarshal
	type Alias KeepaliveConfig
	aux := (*Alias)(k)

	// 2. Decode the YAML into the struct
	if err := value.Decode(aux); err != nil {
		return err
	}

	// 3. Apply your logic: if enabled but interval is missing/zero
	if k.Enabled && k.Interval == 0 {
		k.Interval = 30 * time.Second
	}

	return nil
}

// UnmarshalJSON implements JSON unmarshaling for KeepaliveConfig with defaults.
func (k *KeepaliveConfig) UnmarshalJSON(data []byte) error {
	// Define a alias and auxiliary struct to handle the duration
	type Alias KeepaliveConfig
	aux := &struct {
		Interval string `json:"interval"` // Capture interval as a string
		*Alias
	}{
		Alias: (*Alias)(k),
	}

	// Unmarshal the raw JSON into auxiliary struct
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Maniually parse the duration string if it exists
	if aux.Interval != "" {
		d, err := time.ParseDuration(aux.Interval)
		if err != nil {
			return fmt.Errorf("invalid json duration: %v", err)
		}
		k.Interval = d
	}

	// Apply defaults: if Enabled is true and Interval is zero, set to 30s
	if k.Enabled && k.Interval == 0 {
		k.Interval = 30 * time.Second
	}
	return nil
}

// MarshalJSON implements JSON marshaling for KeepaliveConfig with string interval.
func (k KeepaliveConfig) MarshalJSON() ([]byte, error) {
	type Alias KeepaliveConfig
	aux := &struct {
		Interval string `json:"interval,omitempty"`
		*Alias
	}{
		Interval: func() string {
			if k.Interval > 0 {
				return k.Interval.String()
			}
			return ""
		}(),
		Alias: (*Alias)(&k),
	}
	return json.Marshal(aux)
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
	DialTimeout     time.Duration      // Timeout for establishing outbound connections
	ExternalProxies []ExternalProxy    // Registry of upstream SOCKS5 proxies (injected, not from YAML directly)
	TCPKeepAlive    TCPKeepAliveConfig // Applied to outbound connections
}

// TCPKeepAliveConfig configures TCP keepalive probes on a connection.
//
// Idle is the silent period before the first probe, Interval is the gap between
// probes, and Count is how many unanswered probes are tolerated before the
// connection is dropped. Dead-peer detection therefore takes roughly
// Idle + Interval*Count.
//
// This is distinct from KeepaliveConfig, which drives the SSH-level
// keepalive@openssh.com request.
type TCPKeepAliveConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Idle     time.Duration `yaml:"idle"`
	Interval time.Duration `yaml:"interval"`
	Count    int           `yaml:"count"`
}

// KeepAlive returns the probe parameters to apply, defaulting Idle/Interval
// to 30s and Count to 3 when unset, giving ~1m detection. A zero-value config
// returns Enable=false so callers can skip the syscall entirely.
func (k TCPKeepAliveConfig) KeepAlive() (enabled bool, idle, interval time.Duration, count int) {
	if !k.Enabled {
		return false, 0, 0, 0
	}
	idle, interval, count = k.Idle, k.Interval, k.Count
	if idle <= 0 {
		idle = 30 * time.Second
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if count <= 0 {
		count = 3
	}
	return true, idle, interval, count
}

// ProxyConfig holds the overall proxy server configuration.
type ProxyConfig struct {
	HTTPListen   string
	SOCKS5Listen string
	AdminListen  string
	HTTPSListen  string // Optional TLS-wrapped HTTP proxy
	PACPath      string

	// SOCKS5Auth, when set, requires clients to authenticate with
	// username/password (RFC 1929) instead of allowing no-auth.
	SOCKS5Auth *SOCKS5AuthConfig

	// SOCKS5HandshakeTimeout bounds greeting + auth + CONNECT handling.
	// Zero uses the server default.
	SOCKS5HandshakeTimeout time.Duration

	// TCPKeepAlive configures probes on accepted client connections.
	TCPKeepAlive TCPKeepAliveConfig

	// TLS configuration for HTTPS proxy support
	TLSConfig TLSConfig `yaml:"tls"`
}

// SOCKS5AuthConfig holds credentials required from inbound SOCKS5 clients.
type SOCKS5AuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}
