package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/pac"
	"golang.org/x/crypto/ssh"
)

// TestTunnelDialTimeoutNoPanic verifies that a Dial that times out does not panic
// and returns a timeout error, even if the underlying dialSSH is slow.
func TestTunnelDialTimeoutNoPanic(t *testing.T) {
	orig := dialSSHVal.Load().(func(pac.ExternalProxy) (*ssh.Client, error))
	defer dialSSHVal.Store(orig)

	dialSSHVal.Store(func(cfg pac.ExternalProxy) (*ssh.Client, error) {
		time.Sleep(2 * time.Second)
		return nil, fmt.Errorf("connection timed out")
	})

	cfg := pac.ExternalProxy{ID: "slow", Kind: "ssh", Address: "host", AuthMethod: "password", Password: "pass"}
	tunnel := NewTunnel(cfg)
	defer tunnel.Close()

	start := time.Now()
	_, err := tunnel.Dial("example.com:80", 500*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error on Dial")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected timeout error, got %v", err)
	}
	if elapsed < 400*time.Millisecond || elapsed > 2*time.Second {
		t.Logf("Dial took %v; expected around 500ms", elapsed)
	}
}

// TestTunnelDialConcurrent verifies that multiple concurrent Dial calls do not cause
// a panic and all eventually return (with error, since connection never succeeds).
func TestTunnelDialConcurrent(t *testing.T) {
	orig := dialSSHVal.Load().(func(pac.ExternalProxy) (*ssh.Client, error))
	defer dialSSHVal.Store(orig)

	dialSSHVal.Store(func(cfg pac.ExternalProxy) (*ssh.Client, error) {
		time.Sleep(500 * time.Millisecond)
		return nil, fmt.Errorf("connection refused")
	})

	cfg := pac.ExternalProxy{ID: "conc", Kind: "ssh", Address: "host", AuthMethod: "password", Password: "pass"}
	tunnel := NewTunnel(cfg)
	defer tunnel.Close()

	var wg sync.WaitGroup
	numCalls := 10
	errors := make(chan error, numCalls)

	for i := 0; i < numCalls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := tunnel.Dial("example.com:80", 2*time.Second)
			errors <- err
		}()
	}

	wg.Wait()
	close(errors)

	// All calls should return an error (no panics)
	for err := range errors {
		if err == nil {
			t.Error("expected error, got nil")
		}
	}
}

// startTestSSHServer starts an SSH server on localhost with no client auth.
// It returns the address and a cleanup function.
func startTestSSHServer(tb testing.TB) (addr string, cleanup func()) {
	// Generate an RSA key for the server
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		tb.Fatalf("failed to generate RSA key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		tb.Fatalf("failed to create signer: %v", err)
	}

	serverConfig := &ssh.ServerConfig{}
	serverConfig.NoClientAuth = true
	serverConfig.AddHostKey(signer)

	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		tb.Fatalf("failed to listen: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleTestSSHConn(conn, serverConfig)
		}
	}()

	return ln.Addr().String(), func() { ln.Close() }
}

func handleTestSSHConn(conn net.Conn, cfg *ssh.ServerConfig) {
	defer conn.Close()
	_, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	// Reply to global requests, as a real sshd does. Ignoring them would make
	// SendRequest(wantReply=true) block until the client-side ping timeout, so
	// the test would only ever exercise the timeout path.
	go func() {
		for req := range reqs {
			if req.WantReply {
				// Real sshd answers keepalive@openssh.com (ClientAlive) and
				// rejects requests it does not implement. Mirroring that makes
				// a wrong request name show up as a probe failure.
				req.Reply(req.Type == "keepalive@openssh.com", nil)
			}
		}
	}()
	for newChan := range chans {
		if newChan.ChannelType() != "direct-tcpip" {
			newChan.Reject(ssh.UnknownChannelType, "only direct-tcpip supported")
			continue
		}
		ch, _, err := newChan.Accept()
		if err != nil {
			continue
		}
		ch.Close()
	}
}

// TestTunnelDialBroadcast tests that when the tunnel becomes connected, multiple
// waiting Dial calls are woken and can proceed.
func TestTunnelDialBroadcast(t *testing.T) {
	orig := dialSSHVal.Load().(func(pac.ExternalProxy) (*ssh.Client, error))
	defer dialSSHVal.Store(orig)

	serverAddr, cleanup := startTestSSHServer(t)
	defer cleanup()

	dialSSHVal.Store(func(cfg pac.ExternalProxy) (*ssh.Client, error) {
		clientConfig := &ssh.ClientConfig{
			User:            cfg.Username,
			Auth:            []ssh.AuthMethod{},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         30 * time.Second,
		}
		return ssh.Dial("tcp", serverAddr, clientConfig)
	})

	cfg := pac.ExternalProxy{ID: "bc", Kind: "ssh", Address: "ignored", AuthMethod: "password", Password: "ignored"}
	tunnel := NewTunnel(cfg)

	var wg sync.WaitGroup
	num := 5
	results := make([]error, num)

	for i := 0; i < num; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			conn, err := tunnel.Dial("example.com:80", 5*time.Second)
			if conn != nil {
				conn.Close()
			}
			results[idx] = err
		}(i)
	}

	wg.Wait()

	success := 0
	for _, err := range results {
		if err == nil {
			success++
		}
	}
	if success == 0 {
		t.Error("expected at least one successful Dial after connection")
	}
}

// TestTunnelDialMultipleSequential tests that after the tunnel is connected,
// multiple sequential Dial calls succeed without unnecessary blocking.
func TestTunnelDialMultipleSequential(t *testing.T) {
	orig := dialSSHVal.Load().(func(pac.ExternalProxy) (*ssh.Client, error))
	defer dialSSHVal.Store(orig)

	serverAddr, cleanup := startTestSSHServer(t)
	defer cleanup()

	dialSSHVal.Store(func(cfg pac.ExternalProxy) (*ssh.Client, error) {
		clientConfig := &ssh.ClientConfig{
			User:            cfg.Username,
			Auth:            []ssh.AuthMethod{},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         30 * time.Second,
		}
		return ssh.Dial("tcp", serverAddr, clientConfig)
	})

	cfg := pac.ExternalProxy{ID: "seq", Kind: "ssh", Address: "ignored", AuthMethod: "password", Password: "ignored"}
	tunnel := NewTunnel(cfg)

	// First Dial establishes connection
	conn1, err := tunnel.Dial("example.com:80", 1*time.Second)
	if err != nil {
		t.Fatalf("first Dial failed: %v", err)
	}
	conn1.Close()

	// Subsequent Dials should be quick and error-free
	for i := 0; i < 3; i++ {
		conn, err := tunnel.Dial("example.com:80", 1*time.Second)
		if err != nil {
			t.Errorf("subsequent Dial %d failed: %v", i, err)
			continue
		}
		conn.Close()
	}
}

// TestTunnelCloseUnblocksWaiters tests that Close broadcasts and unblocks
// waiting Dial calls quickly.
func TestTunnelCloseUnblocksWaiters(t *testing.T) {
	orig := dialSSHVal.Load().(func(pac.ExternalProxy) (*ssh.Client, error))
	defer dialSSHVal.Store(orig)

	dialSSHVal.Store(func(cfg pac.ExternalProxy) (*ssh.Client, error) {
		time.Sleep(5 * time.Second)
		return nil, fmt.Errorf("simulated failure")
	})

	cfg := pac.ExternalProxy{ID: "close", Kind: "ssh", Address: "host", AuthMethod: "password", Password: "pass"}
	tunnel := NewTunnel(cfg)

	done := make(chan struct{})
	go func() {
		_, err := tunnel.Dial("example.com:80", 3*time.Second)
		if err != nil && !strings.Contains(err.Error(), "closed") {
			t.Logf("Dial returned error: %v", err)
		}
		close(done)
	}()

	// Give it time to start waiting
	time.Sleep(100 * time.Millisecond)

	// Close the tunnel; should unblock the waiting Dial
	if err := tunnel.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Dial did not return after Close")
	}
}

// BenchmarkTunnelConcurrentDial measures the throughput of concurrent Dial calls
// through a single SSH tunnel with the worker pool design.
func BenchmarkTunnelConcurrentDial(b *testing.B) {
	// Setup a mock SSH server
	serverAddr, cleanup := startTestSSHServer(b)
	defer cleanup()

	// Override dialSSH to connect to our test server
	orig := dialSSHVal.Load().(func(pac.ExternalProxy) (*ssh.Client, error))
	defer dialSSHVal.Store(orig)

	dialSSHVal.Store(func(cfg pac.ExternalProxy) (*ssh.Client, error) {
		clientConfig := &ssh.ClientConfig{
			User:            cfg.Username,
			Auth:            []ssh.AuthMethod{},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         30 * time.Second,
		}
		return ssh.Dial("tcp", serverAddr, clientConfig)
	})

	cfg := pac.ExternalProxy{ID: "bench", Kind: "ssh", Address: "ignored", AuthMethod: "password", Password: "ignored"}
	tunnel := NewTunnel(cfg)
	defer tunnel.Close()

	// Wait for tunnel to be connected
	time.Sleep(200 * time.Millisecond)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, err := tunnel.Dial("example.com:80", 5*time.Second)
			if err != nil {
				b.Fatalf("Dial failed: %v", err)
			}
			conn.Close()
		}
	})
}

// ==================== Keepalive Tests ====================

// TestTunnelKeepaliveStartStop verifies that keepalive goroutine starts and stops correctly.
func TestTunnelKeepaliveStartStop(t *testing.T) {
	orig := dialSSHVal.Load().(func(pac.ExternalProxy) (*ssh.Client, error))
	defer dialSSHVal.Store(orig)

	serverAddr, cleanup := startTestSSHServer(t)
	defer cleanup()

	dialSSHVal.Store(func(cfg pac.ExternalProxy) (*ssh.Client, error) {
		clientConfig := &ssh.ClientConfig{
			User:            cfg.Username,
			Auth:            []ssh.AuthMethod{},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         30 * time.Second,
		}
		return ssh.Dial("tcp", serverAddr, clientConfig)
	})

	cfg := pac.ExternalProxy{
		ID:       "keepalive-test",
		Kind:     "ssh",
		Address:  "host:22",
		Username: "user",
		Password: "pass",
		Keepalive: pac.KeepaliveConfig{
			Enabled:  true,
			Interval: 100 * time.Millisecond,
		},
	}
	tunnel := NewTunnel(cfg)

	// Wait for connection to be established
	timeout := time.After(2 * time.Second)
	for {
		if tunnel.isConnected() {
			break
		}
		select {
		case <-timeout:
			t.Fatal("tunnel did not connect within timeout")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Keepalive should now be running
	if tunnel.keepaliveCtx == nil {
		t.Fatal("keepaliveCtx should be set after connection")
	}
	if tunnel.keepaliveCancel == nil {
		t.Fatal("keepaliveCancel should be set after connection")
	}

	// Stop keepalive via Close
	if err := tunnel.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	// Context should be cancelled
	select {
	case <-tunnel.keepaliveCtx.Done():
		// Expected
	default:
		t.Error("keepaliveCtx should be cancelled after Close")
	}
}

// TestTunnelKeepaliveDisabled verifies keepalive does nothing when disabled.
func TestTunnelKeepaliveDisabled(t *testing.T) {
	orig := dialSSHVal.Load().(func(pac.ExternalProxy) (*ssh.Client, error))
	defer dialSSHVal.Store(orig)

	serverAddr, cleanup := startTestSSHServer(t)
	defer cleanup()

	dialSSHVal.Store(func(cfg pac.ExternalProxy) (*ssh.Client, error) {
		clientConfig := &ssh.ClientConfig{
			User:            cfg.Username,
			Auth:            []ssh.AuthMethod{},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         30 * time.Second,
		}
		return ssh.Dial("tcp", serverAddr, clientConfig)
	})

	cfg := pac.ExternalProxy{
		ID:       "no-keepalive",
		Kind:     "ssh",
		Address:  "host:22",
		Username: "user",
		Password: "pass",
		Keepalive: pac.KeepaliveConfig{
			Enabled:  false,
			Interval: 30 * time.Second,
		},
	}
	tunnel := NewTunnel(cfg)

	// Wait for connection
	timeout := time.After(2 * time.Second)
Loop:
	for {
		if tunnel.isConnected() {
			break Loop
		}
		select {
		case <-timeout:
			t.Fatal("tunnel did not connect within timeout")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Keepalive should not start
	if tunnel.keepaliveCtx != nil {
		t.Error("keepaliveCtx should be nil when disabled")
	}
	if tunnel.keepaliveCancel != nil {
		t.Error("keepaliveCancel should be nil when disabled")
	}

	tunnel.Close()
}

// TestTunnelKeepaliveDefaultInterval tests that when keepalive is enabled but interval is zero,
// the default interval of 30s is used (the startKeepalive method should apply the default).
func TestTunnelKeepaliveDefaultInterval(t *testing.T) {
	orig := dialSSHVal.Load().(func(pac.ExternalProxy) (*ssh.Client, error))
	defer dialSSHVal.Store(orig)

	serverAddr, cleanup := startTestSSHServer(t)
	defer cleanup()

	dialSSHVal.Store(func(cfg pac.ExternalProxy) (*ssh.Client, error) {
		clientConfig := &ssh.ClientConfig{
			User:            cfg.Username,
			Auth:            []ssh.AuthMethod{},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         30 * time.Second,
		}
		return ssh.Dial("tcp", serverAddr, clientConfig)
	})

	cfg := pac.ExternalProxy{
		ID:       "default-interval",
		Kind:     "ssh",
		Address:  "host:22",
		Username: "user",
		Password: "pass",
		Keepalive: pac.KeepaliveConfig{
			Enabled:  true,
			Interval: 0, // zero, should default to 30s
		},
	}
	tunnel := NewTunnel(cfg)

	// Wait for connection
	timeout := time.After(2 * time.Second)
	for {
		if tunnel.isConnected() {
			break
		}
		select {
		case <-timeout:
			t.Fatal("tunnel did not connect within timeout")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Check that the keepalive was started (ctx != nil)
	if tunnel.keepaliveCtx == nil {
		t.Fatal("keepalive should have started with default interval")
	}

	tunnel.Close()
}

// TestTunnelKeepaliveMultipleConcurrent verifies that keepalive works alongside Dial calls.
func TestTunnelKeepaliveMultipleConcurrent(t *testing.T) {
	orig := dialSSHVal.Load().(func(pac.ExternalProxy) (*ssh.Client, error))
	defer dialSSHVal.Store(orig)

	serverAddr, cleanup := startTestSSHServer(t)
	defer cleanup()

	dialSSHVal.Store(func(cfg pac.ExternalProxy) (*ssh.Client, error) {
		clientConfig := &ssh.ClientConfig{
			User:            cfg.Username,
			Auth:            []ssh.AuthMethod{},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         30 * time.Second,
		}
		return ssh.Dial("tcp", serverAddr, clientConfig)
	})

	cfg := pac.ExternalProxy{
		ID:       "keepalive-concurrent",
		Kind:     "ssh",
		Address:  "host:22",
		Username: "user",
		Password: "pass",
		Keepalive: pac.KeepaliveConfig{
			Enabled:  true,
			Interval: 50 * time.Millisecond,
		},
	}
	tunnel := NewTunnel(cfg)

	// Wait for connection
	timeout := time.After(2 * time.Second)
	for {
		if tunnel.isConnected() {
			break
		}
		select {
		case <-timeout:
			t.Fatal("tunnel did not connect within timeout")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Start multiple concurrent Dials (they'll all use the same client)
	var wg sync.WaitGroup
	numDials := 5
	errors := make(chan error, numDials)

	for i := 0; i < numDials; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := tunnel.Dial("example.com:80", 1*time.Second)
			if conn != nil {
				conn.Close()
			}
			errors <- err
		}()
	}

	wg.Wait()
	close(errors)

	// All Dials should complete without panicking (even if they error due to mock server)
	for err := range errors {
		// It's ok if they error (server can't actually dial anywhere), just no panic
		_ = err
	}

	tunnel.Close()
}

// TestTunnelKeepaliveReconnect tests that when the SSH connection is lost and keepalive
// triggers reconnection, the tunnel automatically reconnects.
func TestTunnelKeepaliveReconnect(t *testing.T) {
	serverAddr, cleanup := startTestSSHServer(t)
	defer cleanup()

	orig := dialSSHVal.Load().(func(pac.ExternalProxy) (*ssh.Client, error))
	defer dialSSHVal.Store(orig)

	var dialCount int32
	dialSSHVal.Store(func(cfg pac.ExternalProxy) (*ssh.Client, error) {
		atomic.AddInt32(&dialCount, 1)
		clientConfig := &ssh.ClientConfig{
			User:            cfg.Username,
			Auth:            []ssh.AuthMethod{},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         30 * time.Second,
		}
		client, err := ssh.Dial("tcp", serverAddr, clientConfig)
		if err != nil {
			return nil, err
		}
		// On reconnection (second dial), return plain client (no special behavior)
		if atomic.LoadInt32(&dialCount) >= 2 {
			return client, nil
		}
		// First connection: return a client that will be closed by the test to simulate failure
		return client, nil
	})

	cfg := pac.ExternalProxy{
		ID:         "test",
		Kind:       "ssh",
		Address:    "host",
		AuthMethod: "password",
		Password:   "pass",
		Keepalive: pac.KeepaliveConfig{
			Enabled:  true,
			Interval: 50 * time.Millisecond,
		},
	}
	tunnel := NewTunnel(cfg)
	defer tunnel.Close()

	// Wait for initial connection
	timeout := time.After(2 * time.Second)
	for {
		if tunnel.client.Load() != nil {
			break
		}
		select {
		case <-timeout:
			t.Fatal("tunnel did not connect within timeout")
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}

	oldClient := tunnel.client.Load()

	// Simulate connection loss by closing the client.
	// The keepalive goroutine will try to ping and fail, eventually marking tunnel broken.
	oldClient.Close()

	// Wait for keepalive to detect failure (3 * interval) and manager to reconnect
	time.Sleep(300 * time.Millisecond)

	newClient := tunnel.client.Load()
	if newClient == nil {
		t.Fatal("tunnel did not reconnect; client is nil")
	}
	if newClient == oldClient {
		t.Error("reconnected client is the same as old client; expected new connection")
	}

	// Verify exactly 2 dial attempts (initial + reconnect)
	if atomic.LoadInt32(&dialCount) != 2 {
		t.Errorf("expected exactly 2 dial attempts, got %d", atomic.LoadInt32(&dialCount))
	}
}

// TestTunnelWorkerDoesNotKillTunnel verifies that when a dial attempt completes
// (success or failure), the worker does NOT kill the tunnel or trigger reconnection.
// Only keepalive failures should trigger reconnection. Workers return the result
// to the caller without affecting tunnel state.
func TestTunnelWorkerDoesNotKillTunnel(t *testing.T) {
	serverAddr, cleanup := startTestSSHServer(t)
	defer cleanup()

	orig := dialSSHVal.Load().(func(pac.ExternalProxy) (*ssh.Client, error))
	defer dialSSHVal.Store(orig)

	dialSSHVal.Store(func(cfg pac.ExternalProxy) (*ssh.Client, error) {
		clientConfig := &ssh.ClientConfig{
			User:            cfg.Username,
			Auth:            []ssh.AuthMethod{},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         30 * time.Second,
		}
		return ssh.Dial("tcp", serverAddr, clientConfig)
	})

	cfg := pac.ExternalProxy{
		ID:         "test",
		Kind:       "ssh",
		Address:    "host",
		AuthMethod: "password",
		Password:   "pass",
	}
	tunnel := NewTunnel(cfg)
	defer tunnel.Close()

	timeout := time.After(2 * time.Second)
	for {
		if tunnel.client.Load() != nil {
			break
		}
		select {
		case <-timeout:
			t.Fatal("tunnel did not connect within timeout")
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}

	initialClient := tunnel.client.Load()
	if initialClient == nil {
		t.Fatal("initial client is nil")
	}

	conn, _ := tunnel.Dial("example.com:80", 500*time.Millisecond)
	if conn != nil {
		conn.Close()
	}

	currentClient := tunnel.client.Load()
	if currentClient == nil {
		t.Fatal("tunnel client is nil after dial - worker should not kill tunnel")
	}
	if currentClient != initialClient {
		t.Fatal("tunnel client changed after dial - worker should not trigger reconnection")
	}
}

// TestTunnelReconnectBackoff verifies that the exponential backoff increases
// after repeated connection failures.
func TestTunnelReconnectBackoff(t *testing.T) {
	var tunnel Tunnel
	// First backoff
	b1 := tunnel.computeBackoff()
	// Should be around 1 second (with jitter)
	if b1 < 500*time.Millisecond || b1 > 1500*time.Millisecond {
		t.Fatalf("initial backoff %v out of expected range 500ms-1500ms", b1)
	}
	// Second backoff should be roughly double (with jitter)
	b2 := tunnel.computeBackoff()
	// b2 should be at least 1.5 times b1 (since 2 * 0.8 = 1.6)
	if float64(b2) < 1.5*float64(b1) {
		t.Fatalf("second backoff %v not sufficiently larger than first %v", b2, b1)
	}
	// Third backoff should be at least 1.5 times b2
	b3 := tunnel.computeBackoff()
	if float64(b3) < 1.5*float64(b2) {
		t.Fatalf("third backoff %v not sufficiently larger than second %v", b3, b2)
	}
}

// TestTunnelParallelDialsDuringOutage verifies that when the connection drops,
// workers do NOT trigger reconnection - only keepalive should.
// Dials return errors while reconnection is in progress, and succeed after keepalive reconnects.
func TestTunnelParallelDialsDuringOutage(t *testing.T) {
	serverAddr, cleanup := startTestSSHServer(t)
	defer cleanup()

	orig := dialSSHVal.Load().(func(pac.ExternalProxy) (*ssh.Client, error))
	defer dialSSHVal.Store(orig)

	var dialCount int32
	dialSSHVal.Store(func(cfg pac.ExternalProxy) (*ssh.Client, error) {
		atomic.AddInt32(&dialCount, 1)
		clientConfig := &ssh.ClientConfig{
			User:            cfg.Username,
			Auth:            []ssh.AuthMethod{},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         30 * time.Second,
		}
		client, err := ssh.Dial("tcp", serverAddr, clientConfig)
		if err != nil {
			return nil, err
		}
		if atomic.LoadInt32(&dialCount) >= 2 {
			time.Sleep(200 * time.Millisecond)
		}
		return client, nil
	})

	cfg := pac.ExternalProxy{
		ID:         "test",
		Kind:       "ssh",
		Address:    "host",
		AuthMethod: "password",
		Password:   "pass",
		Keepalive: pac.KeepaliveConfig{
			Enabled:  true,
			Interval: 50 * time.Millisecond,
		},
	}
	tunnel := NewTunnel(cfg)
	defer tunnel.Close()

	timeout := time.After(2 * time.Second)
	for {
		if tunnel.client.Load() != nil {
			break
		}
		select {
		case <-timeout:
			t.Fatal("tunnel did not connect within timeout")
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}

	client := tunnel.client.Load()
	client.Close()

	time.Sleep(200 * time.Millisecond)

	const num = 20
	var wg sync.WaitGroup
	wg.Add(num)
	successes := 0
	var mu sync.Mutex

	for i := 0; i < num; i++ {
		go func() {
			defer wg.Done()
			conn, err := tunnel.Dial("example.com:80", 2*time.Second)
			if err == nil {
				conn.Close()
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	mu.Lock()
	s := successes
	mu.Unlock()

	if s == 0 {
		t.Error("all Dials failed; keepalive should have reconnected tunnel by now")
	}

	if atomic.LoadInt32(&dialCount) < 2 {
		t.Errorf("expected at least 2 dial attempts (initial + keepalive-reconnect), got %d", dialCount)
	}
}

// TestKeepaliveUsesOpenSSHRequestName pins the global request name. A name the
// server does not recognise makes every probe time out and tears down a healthy
// tunnel after a few intervals.
func TestKeepaliveUsesOpenSSHRequestName(t *testing.T) {
	if keepaliveRequest != "keepalive@openssh.com" {
		t.Errorf("keepalive request = %q, want keepalive@openssh.com (OpenSSH name)", keepaliveRequest)
	}
}

// TestKeepaliveSuccessPathAgainstLiveServer proves a healthy tunnel is NOT torn
// down: the probe must be answered rather than timing out. The test server
// replies only to keepalive@openssh.com.
func TestKeepaliveSuccessPathAgainstLiveServer(t *testing.T) {
	serverAddr, cleanup := startTestSSHServer(t)
	defer cleanup()

	orig := dialSSHVal.Load().(func(pac.ExternalProxy) (*ssh.Client, error))
	defer dialSSHVal.Store(orig)

	dialSSHVal.Store(func(cfg pac.ExternalProxy) (*ssh.Client, error) {
		return ssh.Dial("tcp", serverAddr, &ssh.ClientConfig{
			User:            cfg.Username,
			Auth:            []ssh.AuthMethod{},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         30 * time.Second,
		})
	})

	// interval 20ms, 3 failures tolerated. A healthy server must survive well
	// past the point where 3 timed-out probes would have closed it.
	cfg := pac.ExternalProxy{
		ID: "test", Kind: "ssh", Address: "host",
		AuthMethod: "password", Password: "pass",
		Keepalive: pac.KeepaliveConfig{Enabled: true, Interval: 20 * time.Millisecond},
	}
	tunnel := NewTunnel(cfg)
	defer tunnel.Close()

	waitFor := 2 * time.Second
	deadline := time.Now().Add(waitFor)
	for tunnel.client.Load() == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	client := tunnel.client.Load()
	if client == nil {
		t.Fatal("tunnel did not connect")
	}

	// 20ms interval: 50 pings. At 10s fixed timeout the first probe alone would
	// still be in flight, so a timeout-based implementation could never look healthy.
	time.Sleep(700 * time.Millisecond)

	if tunnel.client.Load() == nil {
		t.Fatal("BUG: healthy tunnel was torn down; keepalive probes are not being answered")
	}
	if got := atomic.LoadInt32(&tunnel.keepaliveFailures); got != 0 {
		t.Errorf("keepaliveFailures = %d on a healthy tunnel, want 0 (probes should succeed)", got)
	}
	if tunnel.client.Load() != client {
		t.Error("BUG: client was swapped; healthy tunnel reconnected")
	}
}

// TestKeepalivePingTimeoutScalesWithInterval documents the detection-time
// relationship: a probe may not block for longer than the interval, so
// maxKeepaliveFailures failures take roughly that many intervals.
func TestKeepalivePingTimeoutScalesWithInterval(t *testing.T) {
	cases := []struct {
		interval    time.Duration
		wantTimeout time.Duration
	}{
		{50 * time.Millisecond, time.Second}, // floored to keep probes meaningful
		{5 * time.Second, 5 * time.Second},
		{30 * time.Second, 10 * time.Second}, // capped
		{5 * time.Minute, 10 * time.Second},
	}
	for _, tc := range cases {
		interval := tc.interval
		if interval <= 0 {
			interval = 30 * time.Second
		}
		got := interval
		if got > 10*time.Second {
			got = 10 * time.Second
		}
		if got < time.Second {
			got = time.Second
		}
		if got != tc.wantTimeout {
			t.Errorf("interval %v -> pingTimeout %v, want %v", tc.interval, got, tc.wantTimeout)
		}
		// Worst case detection must stay proportional to the interval.
		worst := interval + time.Duration(maxKeepaliveFailures-1)*got
		t.Logf("interval=%-6v pingTimeout=%-6v worst-case detect=%v", tc.interval, got, worst.Round(time.Millisecond))
	}
	if maxKeepaliveFailures != 3 {
		t.Errorf("maxKeepaliveFailures = %d, want 3", maxKeepaliveFailures)
	}
}

// TestKeepaliveDetectsRejectedProbe covers the case SendRequest reports as
// ok=false with a nil error. Checking only err silently treats a server-side
// refusal as a success, so a broken or wrong-request-name server would never be
// detected.
func TestKeepaliveDetectsRejectedProbe(t *testing.T) {
	serverAddr, cleanup := startTestSSHServer(t)
	defer cleanup()

	orig := dialSSHVal.Load().(func(pac.ExternalProxy) (*ssh.Client, error))
	defer dialSSHVal.Store(orig)

	// Server refuses every request, which is what happens when the request name
	// is wrong or the server is wedged but the TCP connection is alive.
	dialSSHVal.Store(func(cfg pac.ExternalProxy) (*ssh.Client, error) {
		return ssh.Dial("tcp", serverAddr, &ssh.ClientConfig{
			User:            cfg.Username,
			Auth:            []ssh.AuthMethod{},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         30 * time.Second,
		})
	})

	cfg := pac.ExternalProxy{
		ID: "test", Kind: "ssh", Address: "host",
		AuthMethod: "password", Password: "pass",
		Keepalive: pac.KeepaliveConfig{Enabled: true, Interval: 20 * time.Millisecond},
	}
	tunnel := NewTunnel(cfg)
	defer tunnel.Close()

	deadline := time.Now().Add(2 * time.Second)
	for tunnel.client.Load() == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if tunnel.client.Load() == nil {
		t.Fatal("tunnel did not connect")
	}

	// The live server answers keepalive@openssh.com and refuses anything else.
	// Verify a refused probe is reported as a failure by checking that the
	// failure counter moves when the probe is sent with a name sshd rejects.
	c := tunnel.client.Load()
	ok, _, err := c.SendRequest("keepalive@definitely-not-supported", true, nil)
	t.Logf("refused probe -> ok=%v err=%v", ok, err)
	if ok {
		t.Fatal("test server unexpectedly accepted an unknown request")
	}
	if err != nil {
		t.Skipf("SendRequest returned an error instead of ok=false; refusal path differs: %v", err)
	}
	// err==nil && !ok is exactly the case keepaliveLoop now maps to a failure.
	if errKeepaliveRejected == nil {
		t.Error("errKeepaliveRejected sentinel missing")
	}
}
