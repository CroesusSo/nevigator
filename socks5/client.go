package socks5

import (
	"fmt"
	"net"
	"time"

	"github.com/yourorg/proxy-pac/pac"
)

// DialWithAuth establishes a TCP connection to the SOCKS5 server and performs
// the handshake, including optional username/password authentication.
// On success, it returns the connected net.Conn ready for CONNECT request.
func DialWithAuth(network, addr string, auth *Auth, timeout time.Duration) (net.Conn, error) {
	return DialWithAuthKeepAlive(network, addr, auth, timeout, nil)
}

// DialWithAuthKeepAlive behaves like DialWithAuth but applies TCP keepalive
// settings to the dialled connection. Pass a nil config for Go's defaults.
func DialWithAuthKeepAlive(network, addr string, auth *Auth, timeout time.Duration, ka *pac.TCPKeepAliveConfig) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: timeout}
	if ka != nil {
		dialer = ka.Dialer(timeout)
	}
	conn, err := dialer.Dial(network, addr)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	if err := Handshake(conn, auth); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// Handshake performs the SOCKS5 greeting and the optional username/password
// subnegotiation on an established connection.
//
// The connection is owned by the caller: on error the caller must close it.
func Handshake(conn net.Conn, auth *Auth) error {
	// Build method list. When credentials are configured, offer only
	// user/password: offering no-auth as well lets an upstream that
	// impersonates the real one select 0x00 and receive an unauthenticated
	// tunnel that the client believes it authenticated.
	authRequired := auth != nil && (auth.Username != "" || auth.Password != "")
	var methods []byte
	if authRequired {
		methods = []byte{AuthUserPass}
	} else {
		methods = []byte{AuthNone}
	}

	// Write greeting: VER, NMETHODS, METHODS
	if _, err := conn.Write([]byte{Version, byte(len(methods))}); err != nil {
		return fmt.Errorf("write version/nmeth: %w", err)
	}
	if len(methods) > 0 {
		if _, err := conn.Write(methods); err != nil {
			return fmt.Errorf("write methods: %w", err)
		}
	}

	// Read response: VER, METHOD
	resp := make([]byte, 2)
	if err := ReadFull(conn, resp); err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp[0] != Version {
		return fmt.Errorf("unexpected version: %d", resp[0])
	}
	chosen := resp[1]

	switch chosen {
	case AuthNone:
		if authRequired {
			return fmt.Errorf("%w: upstream selected no-auth although credentials were configured",
				ErrAuthFailed)
		}
		// ok
	case AuthUserPass:
		if auth == nil || (auth.Username == "" && auth.Password == "") {
			return fmt.Errorf("server requires username/password but none provided")
		}
		// Send credentials: VER, ULEN, UNAME, PLEN, PASSWD
		u := []byte(auth.Username)
		p := []byte(auth.Password)
		cred := make([]byte, 1+1+len(u)+1+len(p))
		cred[0] = 0x01 // VER
		cred[1] = byte(len(u))
		copy(cred[2:], u)
		offset := 2 + len(u)
		cred[offset] = byte(len(p))
		copy(cred[offset+1:], p)
		if _, err := conn.Write(cred); err != nil {
			return fmt.Errorf("write credentials: %w", err)
		}
		authResp := make([]byte, 2)
		if err := ReadFull(conn, authResp); err != nil {
			return fmt.Errorf("read auth response: %w", err)
		}
		if authResp[1] != REPSuccess {
			return fmt.Errorf("%w: code %d", ErrAuthFailed, authResp[1])
		}
	default:
		if chosen == AuthNoAcceptable {
			return ErrNoAcceptableMethod
		}
		return fmt.Errorf("unsupported auth method: %d", chosen)
	}

	return nil
}
