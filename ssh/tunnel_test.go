package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net"
	"strings"
	"sync"
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
func startTestSSHServer(t *testing.T) (addr string, cleanup func()) {
	// Generate an RSA key for the server
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}

	serverConfig := &ssh.ServerConfig{}
	serverConfig.NoClientAuth = true
	serverConfig.AddHostKey(signer)

	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
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
	// Drain requests to avoid blocking
	go func() {
		for range reqs {
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
