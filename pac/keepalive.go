package pac

import (
	"log"
	"net"
	"time"
)

// ApplyKeepAlive configures TCP keepalive probes on conn according to cfg.
//
// It is a no-op when keepalive is disabled, when conn is not a TCP connection,
// or when the platform rejects the options. Failures are logged rather than
// returned so callers on hot paths (every accepted connection) need no error
// handling: keepalive tuning is best-effort and must never break a tunnel.
//
// Apply it to both dialled and accepted connections. Go enables SO_KEEPALIVE on
// accepted sockets but leaves the probe tuning at 15s/15s/9, which detects a dead
// peer only after ~2m30s.
func ApplyKeepAlive(conn net.Conn, cfg TCPKeepAliveConfig) {
	enabled, idle, interval, count := cfg.KeepAlive()
	if !enabled {
		return
	}
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return
	}
	err := tcpConn.SetKeepAliveConfig(net.KeepAliveConfig{
		Enable:   true,
		Idle:     idle,
		Interval: interval,
		Count:    count,
	})
	if err != nil {
		log.Printf("TCP keepalive not applied: %v", err)
	}
}

// Dialer returns a net.Dialer that applies the given keepalive settings to every
// outbound connection it creates.
//
// When keepalive is disabled the dialer is left at Go's defaults, which enable
// keepalive with a 15s idle period. Disabling it here would silently drop
// keepalive for users who never configured it.
func (k TCPKeepAliveConfig) Dialer(timeout time.Duration) *net.Dialer {
	d := &net.Dialer{Timeout: timeout}
	enabled, idle, interval, count := k.KeepAlive()
	if !enabled {
		return d
	}
	d.KeepAliveConfig = net.KeepAliveConfig{
		Enable:   true,
		Idle:     idle,
		Interval: interval,
		Count:    count,
	}
	return d
}
