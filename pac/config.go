package pac

import (
	"net"
	"time"
)

// ExternalProxy represents an upstream SOCKS5 proxy with optional authentication.
type ExternalProxy struct {
	ID       string `yaml:"id" json:"id"`
	Address  string `yaml:"address" json:"address"`
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
}

// Route represents a routing decision from PAC evaluation.
type Route struct {
	Strategy   string // "TUN", "SOCKS5", "DIRECT", "EXTERNAL", "DEFAULT"
	Address    string // required for SOCKS5, ignored otherwise
	ExternalID string // used when Strategy == "EXTERNAL"
}

// EvaluatorConfig holds configuration for the PAC Evaluator.
type EvaluatorConfig struct {
	PACPath        string        // Path to PAC file
	LocalIP        string        // Value returned by myIpAddress()
	DNSTimeout     time.Duration // Timeout for dnsResolve/isResolvable
	EvalTimeout    time.Duration // Max time for Evaluate()
	EnableDNSCache bool          // Enable in-memory DNS cache
	FallbackRoute  Route         // Used when PAC fails
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
}
