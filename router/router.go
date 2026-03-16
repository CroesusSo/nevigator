package router

import (
	"errors"
	"log"
	"net"
	"time"

	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/socks5"
)

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
	cfg         pac.RouterConfig
	tunIP       net.IP // resolved TUN IP
	dialTimeout time.Duration
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
		return r.dialSOCKS5(dest, route.Address, dialer)
	case "DIRECT":
		return dialer.Dial("tcp", dest)
	default:
		return nil, ErrUnknownStrategy
	}
}

// dialSOCKS5 establishes a connection through a SOCKS5 server.
func (r *Router) dialSOCKS5(destAddr, socksAddr string, dialer *net.Dialer) (net.Conn, error) {
	conn, err := dialer.Dial("tcp", socksAddr)
	if err != nil {
		return nil, err
	}

	// Greeting: VER=5, NMETHODS=1, METHOD=NO_AUTH
	_, err = conn.Write([]byte{socks5.Version, 0x01, socks5.AuthNone})
	if err != nil {
		conn.Close()
		return nil, err
	}

	// Read method selection
	resp := make([]byte, 2)
	if err := socks5.ReadFull(conn, resp); err != nil {
		conn.Close()
		return nil, err
	}
	if resp[0] != socks5.Version || resp[1] != socks5.AuthNone {
		conn.Close()
		return nil, ErrSOCKS5AuthFailed
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
	resp = make([]byte, 4)
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

	return conn, nil
}
