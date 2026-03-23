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
	ErrTUNNotConfigured              = errors.New("TUN not configured")
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
	tunIP          net.IP // resolved TUN IP
	dialTimeout    time.Duration
	proxyMu        sync.RWMutex
	proxyRegistry  map[string]pac.ExternalProxy // ID → proxy
	defaultProxyID string
	tunnelMgr      TunnelManager // optional, for SSH proxies
}

// NewRouter creates a Router with the given config.
// It also discovers the TUN IP if device is configured.
func NewRouter(cfg pac.RouterConfig) (*Router, error) {
	r := &Router{
		cfg:         cfg,
		dialTimeout: cfg.DialTimeout,
	}
	if cfg.DialTimeout == 0 {
		r.dialTimeout = 30 * time.Second
	}
	r.tunIP = cfg.TUNIP
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
// If the route fails (and is not DIRECT), it automatically retries with DIRECT.
func (r *Router) Route(dest string, route pac.Route) (net.Conn, error) {
	conn, err := r.dialWithRoute(dest, route)
	if err != nil && route.Strategy != "DIRECT" {
		log.Printf("Route %s failed: %v; falling back to DIRECT", route.Strategy, err)
		conn, err = r.dialWithRoute(dest, pac.Route{Strategy: "DIRECT"})
	}
	return conn, err
}

// dialWithRoute dials the destination using the specified route.
func (r *Router) dialWithRoute(dest string, route pac.Route) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   r.dialTimeout,
		LocalAddr: nil,
	}

	switch route.Strategy {
	case "TUN":
		if r.tunIP == nil {
			return nil, ErrTUNNotConfigured
		}
		dialer.LocalAddr = &net.TCPAddr{
			IP:   r.tunIP,
			Port: 0,
		}
		return dialer.Dial("tcp", dest)
	case "SOCKS5":
		if route.Address == "" {
			return nil, ErrInvalidSOCKS5Address
		}
		return r.dialSOCKS5WithAuth(dest, route.Address, dialer, nil)
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
			return r.dialSOCKS5WithAuth(dest, proxy.Address, dialer, auth)
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
			return r.dialSOCKS5WithAuth(dest, proxy.Address, dialer, auth)
		}
	case "DIRECT":
		return dialer.Dial("tcp", dest)
	default:
		return nil, ErrUnknownStrategy
	}
}

// dialSOCKS5WithAuth establishes a connection through a SOCKS5 server with optional authentication.
func (r *Router) dialSOCKS5WithAuth(destAddr, socksAddr string, dialer *net.Dialer, auth *socks5.Auth) (net.Conn, error) {
	// Use socks5.DialWithAuth to handle TCP dial + SOCKS5 handshake
	conn, err := socks5.DialWithAuth("tcp", socksAddr, auth, r.dialTimeout)
	if err != nil {
		return nil, err
	}

	// Build CONNECT request: VER, CMD, RSV, ATYP, ADDR, PORT
	destHost, destPortStr, err := net.SplitHostPort(destAddr)
	if err != nil {
		conn.Close()
		return nil, err
	}
	destPort, _ := net.LookupPort("tcp", destPortStr)
	addrBytes := socks5.SerializeAddr(destHost, uint16(destPort))
	req := make([]byte, 3+len(addrBytes))
	req[0] = socks5.Version
	req[1] = socks5.CMDConnect
	req[2] = 0x00 // RSV
	copy(req[3:], addrBytes)

	_, err = conn.Write(req)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// Read reply: VER, REP, RSV, ATYP, BND.ADDR, BND.PORT
	resp := make([]byte, 10)
	if err := socks5.ReadFull(conn, resp); err != nil {
		conn.Close()
		return nil, err
	}
	if resp[0] != socks5.Version {
		conn.Close()
		return nil, ErrSOCKS5InvalidVersion
	}
	if resp[1] != socks5.REPSuccess {
		conn.Close()
		if err, ok := socks5Errors[resp[1]]; ok {
			return nil, err
		}
		return nil, ErrSOCKS5GeneralFailure
	}

	// BND.ADDR and BND.PORT are ignored as per SOCKS5 spec
	// but we must consume them to leave the connection in clean state

	return conn, nil
}
