package pac

import (
	"net"
	"syscall"
	"testing"
	"time"
)

// TestOutboundKeepAliveNotDisabledByDefault guards a regression: when
// tcp_keepalive is unset, outbound dials must keep Go's implicit keepalive
// rather than having it switched off.
func TestOutboundKeepAliveNotDisabledByDefault(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ch := make(chan *net.TCPConn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		ch <- c.(*net.TCPConn)
	}()

	// zero-value config == user did not set tcp_keepalive
	d := TCPKeepAliveConfig{}.Dialer(2 * time.Second)
	t.Logf("Dialer.KeepAlive = %d (0=Go default 15s, -1=disabled)", d.KeepAlive)
	t.Logf("Dialer.KeepAliveConfig.Enable = %v", d.KeepAliveConfig.Enable)

	conn, err := d.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	sc := <-ch
	defer sc.Close()

	raw, _ := conn.(*net.TCPConn).SyscallConn()
	var on int
	raw.Control(func(fd uintptr) {
		on, _ = syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_KEEPALIVE)
	})
	t.Logf("SO_KEEPALIVE on outbound socket = %d (0 = keepalive OFF)", on)
	if on == 0 {
		t.Error("regression: keepalive disabled on outbound conn; Go default is enabled")
	}
}
