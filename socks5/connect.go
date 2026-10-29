package socks5

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"
)

// Connect sends a SOCKS5 CONNECT request for host:port over an established
// connection and returns the server's reply code.
//
// The destination is sent as a domain name whenever host is not an IP
// literal, so the SOCKS5 server resolves it. That keeps name resolution on
// the far side of the tunnel, which is what a censored network needs.
func Connect(conn net.Conn, host string, port uint16) (byte, error) {
	addrBytes, err := SerializeAddr(host, port)
	if err != nil {
		return 0, fmt.Errorf("serialize dest %s: %w", host, err)
	}
	req := make([]byte, 3+len(addrBytes))
	req[0] = Version
	req[1] = CMDConnect
	req[2] = 0x00 // RSV
	copy(req[3:], addrBytes)
	if _, err := conn.Write(req); err != nil {
		return 0, err
	}
	return ReadReply(conn)
}

// DialConnect establishes a tunnel to destAddr ("host:port") through the
// SOCKS5 server at proxyAddr.
//
// It performs the greeting, the optional authentication and the CONNECT
// request. On success the returned connection carries the tunnelled stream.
func DialConnect(ctx context.Context, proxyAddr, destAddr string, auth *Auth, timeout time.Duration) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(destAddr)
	if err != nil {
		return nil, fmt.Errorf("split dest %s: %w", destAddr, err)
	}
	// The port is encoded in two bytes, so an out-of-range value must be
	// rejected rather than truncated.
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("parse dest port %s: %w", portStr, err)
	}

	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("dial socks5 proxy %s: %w", proxyAddr, err)
	}

	// The handshake and the CONNECT request must not outlive the caller's
	// deadline. The deadline is cleared afterwards: the tunnel is long-lived
	// and carries no deadline of its own.
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	} else if timeout > 0 {
		conn.SetDeadline(time.Now().Add(timeout))
	}

	// A cancelled context must abort the handshake even when it carries no
	// deadline: otherwise the caller waits forever on a peer that never
	// answers, and the goroutine and socket leak.
	stopWatch := watchContext(ctx, conn)
	defer stopWatch()

	if err := Handshake(conn, auth); err != nil {
		conn.Close()
		return nil, err
	}
	rep, err := Connect(conn, host, uint16(port))
	if err != nil {
		conn.Close()
		return nil, err
	}
	if rep != REPSuccess {
		conn.Close()
		return nil, fmt.Errorf("socks5 connect to %s refused: reply %d", destAddr, rep)
	}

	conn.SetDeadline(time.Time{})
	return conn, nil
}
