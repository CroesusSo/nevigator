package pac

import (
	"net"
	"time"
)

// Route represents a routing decision from PAC evaluation.
type Route struct {
	Strategy string // "TUN", "SOCKS5", "DIRECT"
	Address  string // required for SOCKS5, ignored otherwise
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
	TUNDevice   string
	TUNIP       net.IP        // If nil, will be discovered from TUNDevice
	DialTimeout time.Duration // Timeout for establishing outbound connections
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
