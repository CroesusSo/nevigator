package router

import (
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/socks5"
	"github.com/yourorg/proxy-pac/status"
)

// TunnelManager abstraction for dialing through external proxies (SSH or future types).
type TunnelManager interface {
	Dial(id, dest string, timeout time.Duration) (net.Conn, error)
	UpdateProxies(proxies []pac.ExternalProxy)
	Close() error
	GetTunnelsStatus() map[string]status.SSHTunnelStatus
}

var (
	ErrInvalidSOCKS5Address          = errors.New("SOCKS5 address required")
	ErrUnknownStrategy               = errors.New("unknown route strategy")
	ErrSOCKS5AuthFailed              = errors.New("SOCKS5 authentication failed")
	ErrSOCKS5InvalidVersion          = errors.New("SOCKS5 invalid version")
	ErrSOCKS5GeneralFailure          = errors.New("SOCKS5 general failure")
	ErrSOCKS5ConnectionNotAllowed    = errors.New("SOCKS5 connection not allowed")
	ErrSOCKS5NetworkUnreachable      = errors.New("SOCKS5 network unreachable")
	ErrSOCKS5HostUnreachable         = errors.New("SOCKS5 host unreachable")
	ErrSOCKS5ConnectionRefused       = errors.New("SOCKS5 connection refused")
	ErrSOCKS5TTLExpired              = errors.New("SOCKS5 TTL expired")
	ErrSOCKS5CommandNotSupported     = errors.New("SOCKS5 command not supported")
	ErrSOCKS5AddressTypeNotSupported = errors.New("SOCKS5 address type not supported")
)

var socks5Errors = map[byte]error{
	socks5.REPGeneralFailure:       ErrSOCKS5GeneralFailure,
	socks5.REPConnectionNotAllowed: ErrSOCKS5ConnectionNotAllowed,
	socks5.REPNetworkUnreachable:   ErrSOCKS5NetworkUnreachable,
	socks5.REPHostUnreachable:      ErrSOCKS5HostUnreachable,
	socks5.REPConnectionRefused:    ErrSOCKS5ConnectionRefused,
	socks5.REPTTLExpired:           ErrSOCKS5TTLExpired,
	socks5.REPCommandNotSupported:  ErrSOCKS5CommandNotSupported,
	socks5.REPAddrTypeNotSupported: ErrSOCKS5AddressTypeNotSupported,
}

// Router implements the routing logic.
type Router struct {
	cfg            pac.RouterConfig
	dialTimeout    time.Duration
	proxyMu        sync.RWMutex
	proxyRegistry  map[string]pac.ExternalProxy // ID → proxy
	defaultProxyID string
	tunnelMgr      TunnelManager // optional, for SSH proxies

	// customResolver resolves the host names of DIRECT connections. It is
	// nil when no custom DNS servers are configured.
	customResolver *net.Resolver
}

// NewRouter creates a Router with the given config.
func NewRouter(cfg pac.RouterConfig) (*Router, error) {
	r := &Router{
		cfg:         cfg,
		dialTimeout: cfg.DialTimeout,
	}
	if cfg.DialTimeout == 0 {
		r.dialTimeout = 30 * time.Second
	}
	if len(cfg.CustomDNSServers) > 0 {
		r.customResolver = pac.CreateCustomResolver(cfg.CustomDNSServers)
		log.Printf("[Router] Using custom DNS: %v", cfg.CustomDNSServers)
	}
	return r, nil
}

// SetProxyRegistry updates the external proxy registry.
func (r *Router) SetProxyRegistry(proxies []pac.ExternalProxy) {
	r.proxyMu.Lock()
	defer r.proxyMu.Unlock()
	r.proxyRegistry = make(map[string]pac.ExternalProxy, len(proxies))
	for _, p := range proxies {
		r.proxyRegistry[p.ID] = p
	}
	// Notify tunnel manager if present
	if r.tunnelMgr != nil {
		r.tunnelMgr.UpdateProxies(proxies)
	}
}

// SetDefaultProxyID sets the default external proxy ID.
func (r *Router) SetDefaultProxyID(id string) {
	r.proxyMu.Lock()
	defer r.proxyMu.Unlock()
	r.defaultProxyID = id
}

// SetTunnelManager sets the SSH tunnel manager (optional).
func (r *Router) SetTunnelManager(mgr TunnelManager) {
	r.tunnelMgr = mgr
}

// GetTunnelManager returns the configured tunnel manager (may be nil).
func (r *Router) GetTunnelManager() TunnelManager {
	return r.tunnelMgr
}

// Route establishes an outbound connection according to the given route.
// If a non-DIRECT route fails, it retries once via DIRECT.
func (r *Router) Route(dest string, route pac.Route) (net.Conn, error) {
	conn, err := r.dialWithRoute(dest, route)
	if err == nil {
		return conn, nil
	}
	// A DIRECT route has no fallback, and deterministic config errors will not
	// be fixed by retrying.
	if route.Strategy == "DIRECT" || isDeterministic(err) {
		return nil, err
	}

	log.Printf("Route %s via %s failed (%v), falling back to DIRECT", dest, route.Strategy, err)
	fallbackConn, fallbackErr := r.dialDirect(dest)
	if fallbackErr != nil {
		return nil, fmt.Errorf("%w (direct fallback: %v)", err, fallbackErr)
	}
	return fallbackConn, nil
}

// dialDirect establishes a DIRECT connection with the configured dial timeout.
//
// When custom DNS servers are configured they resolve the host name, because
// a poisoned system resolver is a common way to break a direct connection.
// They are authoritative: a name they do not know is not retried against the
// system resolver, which would undo the point of configuring them.
func (r *Router) dialDirect(dest string) (net.Conn, error) {
	dialer := r.cfg.TCPKeepAlive.Dialer(r.dialTimeout)
	if r.customResolver != nil {
		dialer.Resolver = r.customResolver
	}
	return dialer.Dial("tcp", dest)
}

// isDeterministic reports whether err reflects a configuration or protocol
// problem that will repeat identically on every attempt.
func isDeterministic(err error) bool {
	switch {
	case errors.Is(err, ErrUnknownStrategy),
		errors.Is(err, ErrSOCKS5AuthFailed),
		errors.Is(err, ErrInvalidSOCKS5Address),
		errors.Is(err, socks5.ErrDomainTooLong),
		errors.Is(err, socks5.ErrEmptyDomain):
		return true
	}
	// Any non-nil SOCKS5 reply code means the upstream answered and refused.
	for code := range socks5Errors {
		if errors.Is(err, socks5Errors[code]) {
			return true
		}
	}
	return false
}

// dialWithRoute dials the destination using the specified route.
func (r *Router) dialWithRoute(dest string, route pac.Route) (net.Conn, error) {
	switch route.Strategy {
	case "SOCKS5":
		if route.Address == "" {
			return nil, ErrInvalidSOCKS5Address
		}
		return r.dialSOCKS5WithAuth(dest, route.Address, nil)
	case "EXTERNAL":
		if route.ExternalID == "" {
			return nil, ErrInvalidSOCKS5Address
		}
		r.proxyMu.RLock()
		proxy, ok := r.proxyRegistry[route.ExternalID]
		r.proxyMu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("external proxy not found: %s", route.ExternalID)
		}
		switch proxy.Kind {
		case "ssh":
			if r.tunnelMgr == nil {
				return nil, errors.New("SSH tunnel manager not configured")
			}
			// Use the router's dialTimeout for the tunnel Dial
			return r.tunnelMgr.Dial(proxy.ID, dest, r.dialTimeout)
		default: // "socks5" (default)
			auth := &socks5.Auth{Username: proxy.Username, Password: proxy.Password}
			return r.dialSOCKS5WithAuth(dest, proxy.Address, auth)
		}
	case "DEFAULT":
		if route.ExternalID != "" {
			// Should not happen, but guard
			return nil, ErrInvalidSOCKS5Address
		}
		r.proxyMu.RLock()
		id := r.defaultProxyID
		r.proxyMu.RUnlock()
		if id == "" {
			return nil, errors.New("no default external proxy configured")
		}
		r.proxyMu.RLock()
		proxy, ok := r.proxyRegistry[id]
		r.proxyMu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("default external proxy not found: %s", id)
		}
		switch proxy.Kind {
		case "ssh":
			if r.tunnelMgr == nil {
				return nil, errors.New("SSH tunnel manager not configured")
			}
			return r.tunnelMgr.Dial(proxy.ID, dest, r.dialTimeout)
		default: // "socks5"
			auth := &socks5.Auth{Username: proxy.Username, Password: proxy.Password}
			return r.dialSOCKS5WithAuth(dest, proxy.Address, auth)
		}
	case "DIRECT":
		return r.dialDirect(dest)
	default:
		return nil, ErrUnknownStrategy
	}
}

// dialSOCKS5WithAuth establishes a connection through a SOCKS5 server with optional authentication.
func (r *Router) dialSOCKS5WithAuth(destAddr, socksAddr string, auth *socks5.Auth) (net.Conn, error) {
	// Use socks5.DialWithAuth to handle TCP dial + SOCKS5 handshake
	conn, err := socks5.DialWithAuthKeepAlive("tcp", socksAddr, auth, r.dialTimeout, &r.cfg.TCPKeepAlive)
	if err != nil {
		// An upstream that rejects our credentials, or refuses every method
		// we offered, has answered definitively. Retrying DIRECT would send
		// traffic that the route deliberately sent through a proxy straight
		// out of the local network, so it must be reported as-is.
		if errors.Is(err, socks5.ErrAuthFailed) || errors.Is(err, socks5.ErrNoAcceptableMethod) {
			return nil, fmt.Errorf("%w: %v", ErrSOCKS5AuthFailed, err)
		}
		return nil, err
	}

	// Build CONNECT request: VER, CMD, RSV, ATYP, ADDR, PORT
	destHost, destPortStr, err := net.SplitHostPort(destAddr)
	if err != nil {
		conn.Close()
		return nil, err
	}
	destPort, err := net.LookupPort("tcp", destPortStr)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("resolve dest port: %w", err)
	}
	addrBytes, err := socks5.SerializeAddr(destHost, uint16(destPort))
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("serialize dest %s: %w", destHost, err)
	}
	req := make([]byte, 3+len(addrBytes))
	req[0] = socks5.Version
	req[1] = socks5.CMDConnect
	req[2] = 0x00 // RSV
	copy(req[3:], addrBytes)

	// The CONNECT exchange must be bounded as well: an upstream that
	// completes the handshake and then stops answering would block here
	// forever.
	conn.SetDeadline(time.Now().Add(r.dialTimeout))

	_, err = conn.Write(req)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// The reply is variable length: read the header, then consume BND.ADDR and
	// BND.PORT according to ATYP so the stream is left at application data.
	rep, err := socks5.ReadReply(conn)
	if err != nil {
		conn.Close()
		if errors.Is(err, socks5.ErrBadReplyVersion) {
			return nil, ErrSOCKS5InvalidVersion
		}
		return nil, err
	}
	if rep != socks5.REPSuccess {
		conn.Close()
		if e, ok := socks5Errors[rep]; ok {
			return nil, e
		}
		return nil, ErrSOCKS5GeneralFailure
	}

	// The tunnel is long-lived and carries no deadline of its own.
	conn.SetDeadline(time.Time{})
	return conn, nil
}
