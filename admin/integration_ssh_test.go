//go:build integration
// +build integration

package admin

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

var (
	testSSHAddr string
	stopTestSSH func()
)

// TestMain starts/stops the test SSH server for integration tests.
func TestMain(m *testing.M) {
	var err error
	testSSHAddr, stopTestSSH, err = startTestSSHServer()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to start test SSH server: %v; integration tests will be skipped\n", err)
		testSSHAddr = "" // mark unavailable
	}
	code := m.Run()
	if stopTestSSH != nil {
		stopTestSSH()
	}
	os.Exit(code)
}

// startTestSSHServer launches a minimal SSH server on 127.0.0.1:0 (random port).
// It accepts password authentication (any username, password: "testpass").
// For any direct-tcpip channel request, it accepts and immediately closes the channel.
// Returns the server address (e.g., "127.0.0.1:54321") and a stop function.
func startTestSSHServer() (addr string, stop func(), err error) {
	// Generate host key (RSA 2048)
	hostKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", nil, fmt.Errorf("generate host key: %w", err)
	}
	signer, err := ssh.NewSigner(hostKey)
	if err != nil {
		return "", nil, fmt.Errorf("create signer: %w", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("listen: %w", err)
	}
	addr = ln.Addr().String()

	serverConfig := &ssh.ServerConfig{
		HostKey: signer,
		PasswordAuth: func(password []byte) bool {
			return string(password) == "testpass"
		},
	}

	// Accept loop
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go handleSSHConn(conn, serverConfig)
		}
	}()

	stop = func() {
		ln.Close()
	}
	return addr, stop, nil
}

// handleSSHConn performs the SSH handshake and handles channel requests.
func handleSSHConn(conn net.Conn, config *ssh.ServerConfig) {
	defer conn.Close()
	_, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	// Discard global requests (e.g., keepalive)
	go ssh.DiscardRequests(reqs)
	// Handle incoming channel requests
	for newChannel := range chans {
		if newChannel.ChannelType() != "direct-tcpip" {
			newChannel.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		// Immediately close the channel to indicate success (the client gets a connection object).
		channel.Close()
		// Discard channel-specific requests
		go ssh.DiscardRequests(requests)
	}
}

// TestHandleTestProxy_IntegrationSSH tests the full flow of an unsaved SSH proxy test
// using a real in-memory SSH server.
func TestHandleTestProxy_IntegrationSSH(t *testing.T) {
	if testSSHAddr == "" {
		t.Skip("SSH test server not available")
	}
	// Build request payload with host_key_check ignored to avoid known_hosts dependency.
	body := fmt.Sprintf(`{"address":"%s","kind":"ssh","auth_method":"password","password":"testpass","username":"testuser","host_key_check":"ignore"}`,
		testSSHAddr)
	req := httptest.NewRequest(http.MethodPost, "/api/proxies/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h := &Handler{}
	h.handleTestProxy(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if success, ok := resp["success"]; !ok || success != true {
		t.Fatalf("expected success=true, got %v", resp)
	}
}
