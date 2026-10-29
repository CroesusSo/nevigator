package socks5

import (
	"context"
	"net"
	"net/http"
	"time"
)

// HTTPTransportOptions configures the HTTP transport that tunnels through a
// SOCKS5 server.
type HTTPTransportOptions struct {
	// ProxyAddr is the address of the SOCKS5 server to tunnel through.
	ProxyAddr string
	// Auth holds the optional username/password credentials of that server.
	Auth *Auth
	// Timeout bounds the handshake and the CONNECT request.
	Timeout time.Duration
}

// NewHTTPTransport returns an http.Transport that tunnels every request
// through the SOCKS5 server at opts.ProxyAddr.
//
// The destination name is sent to the server unresolved, so the server
// resolves it and applies the same routing rules it applies to any other
// client. Environment proxy settings are ignored: the tunnel is the route.
func NewHTTPTransport(opts HTTPTransportOptions) *http.Transport {
	return &http.Transport{
		// The tunnel already reaches the destination, so the transport must
		// not apply an HTTP proxy of its own.
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return DialConnect(ctx, opts.ProxyAddr, addr, opts.Auth, opts.Timeout)
		},
	}
}
