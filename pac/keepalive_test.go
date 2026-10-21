package pac

import (
	"net"
	"syscall"
	"testing"
	"time"
)

func TestKeepAliveDefaults(t *testing.T) {
	tests := []struct {
		name     string
		cfg      TCPKeepAliveConfig
		wantEn   bool
		wantIdle time.Duration
		wantInt  time.Duration
		wantCnt  int
	}{
		{"disabled zero value", TCPKeepAliveConfig{}, false, 0, 0, 0},
		{"enabled uses defaults", TCPKeepAliveConfig{Enabled: true}, true, 15 * time.Second, 5 * time.Second, 3},
		{"explicit values kept", TCPKeepAliveConfig{Enabled: true, Idle: 5 * time.Second, Interval: time.Second, Count: 2}, true, 5 * time.Second, time.Second, 2},
		{"partial overrides defaults", TCPKeepAliveConfig{Enabled: true, Idle: 60 * time.Second}, true, 60 * time.Second, 5 * time.Second, 3},
		{"zero count falls back", TCPKeepAliveConfig{Enabled: true, Count: 0}, true, 15 * time.Second, 5 * time.Second, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			en, idle, interval, cnt := tt.cfg.KeepAlive()
			if en != tt.wantEn {
				t.Fatalf("enabled = %v, want %v", en, tt.wantEn)
			}
			if !en {
				return
			}
			if idle != tt.wantIdle || interval != tt.wantInt || cnt != tt.wantCnt {
				t.Errorf("got idle=%v interval=%v count=%d; want %v/%v/%d",
					idle, interval, cnt, tt.wantIdle, tt.wantInt, tt.wantCnt)
			}
		})
	}
}

// A zero-value config means the user never set tcp_keepalive. The dialer must
// then keep Go's defaults, which still enable keepalive — disabling it would
// silently regress every unconfigured deployment.
func TestDialerUnconfiguredKeepsGoDefaults(t *testing.T) {
	d := TCPKeepAliveConfig{}.Dialer(5 * time.Second)
	if d.Timeout != 5*time.Second {
		t.Errorf("timeout = %v", d.Timeout)
	}
	if d.KeepAliveConfig.Enable {
		t.Error("keepalive should not be force-enabled")
	}
	if d.KeepAlive < 0 {
		t.Errorf("KeepAlive = %v; must not disable Go's implicit keepalive", d.KeepAlive)
	}
}

func TestDialerEnabled(t *testing.T) {
	d := TCPKeepAliveConfig{Enabled: true, Idle: 30 * time.Second, Interval: 10 * time.Second, Count: 3}.Dialer(time.Second)
	kac := d.KeepAliveConfig
	if !kac.Enable {
		t.Fatal("keepalive not enabled")
	}
	if kac.Idle != 30*time.Second || kac.Interval != 10*time.Second || kac.Count != 3 {
		t.Errorf("got %+v", kac)
	}
}

func tcpKeepAliveOpts(t *testing.T, c *net.TCPConn) (idle, interval, count int) {
	t.Helper()
	raw, err := c.SyscallConn()
	if err != nil {
		t.Fatalf("SyscallConn: %v", err)
	}
	var ierr error
	err = raw.Control(func(fd uintptr) {
		idle, ierr = syscall.GetsockoptInt(int(fd), syscall.IPPROTO_TCP, 4) // TCP_KEEPIDLE
		if ierr != nil {
			return
		}
		interval, _ = syscall.GetsockoptInt(int(fd), syscall.IPPROTO_TCP, 5) // TCP_KEEPINTVL
		count, _ = syscall.GetsockoptInt(int(fd), syscall.IPPROTO_TCP, 6)    // TCP_KEEPCNT
	})
	if err != nil {
		t.Fatalf("Control: %v", err)
	}
	return idle, interval, count
}

// ApplyKeepAlive must retune an accepted socket away from Go's 15s/15s/9 default.
func TestApplyKeepAliveRetunesAcceptedConn(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	type pair struct {
		server *net.TCPConn
	}
	got := make(chan pair, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		got <- pair{server: c.(*net.TCPConn)}
	}()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	p := <-got
	defer p.server.Close()

	beforeIdle, beforeInt, beforeCnt := tcpKeepAliveOpts(t, p.server)

	ApplyKeepAlive(p.server, TCPKeepAliveConfig{Enabled: true, Idle: 30 * time.Second, Interval: 10 * time.Second, Count: 3})

	afterIdle, afterInt, afterCnt := tcpKeepAliveOpts(t, p.server)
	t.Logf("before: idle=%d intvl=%d cnt=%d (Go default)", beforeIdle, beforeInt, beforeCnt)
	t.Logf("after:  idle=%d intvl=%d cnt=%d (applied)", afterIdle, afterInt, afterCnt)

	if afterIdle != 30 || afterInt != 10 || afterCnt != 3 {
		t.Errorf("socket opts not applied: idle=%d intvl=%d cnt=%d", afterIdle, afterInt, afterCnt)
	}
	// The applied config must detect faster than Go's default would.
	beforeTotal := time.Duration(beforeIdle)*time.Second + time.Duration(beforeInt*beforeCnt)*time.Second
	afterTotal := time.Duration(afterIdle)*time.Second + time.Duration(afterInt*afterCnt)*time.Second
	if afterTotal >= beforeTotal {
		t.Errorf("detection time not improved: before=%v after=%v", beforeTotal, afterTotal)
	}
}

// A non-TCP conn must be ignored rather than panicking.
func TestApplyKeepAliveNonTCP(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	ApplyKeepAlive(server, TCPKeepAliveConfig{Enabled: true}) // must not panic
}

func TestApplyKeepAliveDisabled(t *testing.T) {
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
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	sc := <-ch
	defer sc.Close()

	ApplyKeepAlive(sc, TCPKeepAliveConfig{Enabled: false})
	idle, interval, cnt := tcpKeepAliveOpts(t, sc)
	t.Logf("disabled config left opts at idle=%d intvl=%d cnt=%d", idle, interval, cnt)
	if idle == 30 && interval == 10 && cnt == 3 {
		t.Error("disabled config should not have applied the tuned values")
	}
}

// Dialer().Dial must actually produce a socket with the configured probes.
func TestDialerAppliesToOutboundConn(t *testing.T) {
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

	cfg := TCPKeepAliveConfig{Enabled: true, Idle: 30 * time.Second, Interval: 10 * time.Second, Count: 3}
	conn, err := cfg.Dialer(2*time.Second).Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	sc := <-ch
	defer sc.Close()

	// The dialer configures the dialled (client) side; the accepted side keeps
	// Go's defaults. Check the client socket.
	idle, interval, cnt := tcpKeepAliveOpts(t, conn.(*net.TCPConn))
	t.Logf("dialled socket: idle=%d intvl=%d cnt=%d", idle, interval, cnt)
	if idle != 30 || interval != 10 || cnt != 3 {
		t.Errorf("outbound dial did not apply config: idle=%d intvl=%d cnt=%d", idle, interval, cnt)
	}
}
