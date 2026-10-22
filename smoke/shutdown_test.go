// Shutdown check: starts the built binary, sends it SIGTERM and verifies
// what it does while it is still on its way down.
//
// The unit tests in package proxy cover Server.Stop() in isolation. They
// cannot cover the process-level contract: that the binary stops serving
// SOCKS5 immediately on SIGTERM, and that it still exits even when an
// admin request is still in flight.
//
// Run with: make smoke
package smoke

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// startHangServer listens and accepts connections without ever answering,
// so a request to it blocks until the client's own timeout.
func startHangServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		var held []net.Conn
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			held = append(held, c) // hold it open, never respond
		}
	}()
	return ln.Addr().String()
}

// writeShutdownConfig writes a config whose GFW source never answers, so a
// GFW fetch stays in flight for as long as the caller wants.
func writeShutdownConfig(t *testing.T, dir string, socksPort, adminPort int, gfwSource string) string {
	t.Helper()
	cfg := fmt.Sprintf(`proxy:
  http_listen: "127.0.0.1:0"
  socks5_listen: "127.0.0.1:%d"
  admin_listen: "127.0.0.1:%d"
  pac_path: "%s"
pac:
  eval_timeout: 1s
gfw:
  disabled: false
  source_url: "%s"
  local_rules_path: "%s"
  user_rules_path: "%s"
  fetch_on_startup: false
  update_interval: 0s
  http_timeout: 120s
`, socksPort, adminPort, filepath.Join(dir, "proxy.pac"), gfwSource,
		filepath.Join(dir, "gfw-rules.json"), filepath.Join(dir, "user-rules.json"))

	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	return path
}

// socks5Greets reports whether the SOCKS5 port completes a no-auth greeting.
func socks5Greets(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return false
	}
	// Read exactly the two reply bytes. The server holds the connection open
	// waiting for the CONNECT request, so reading to EOF would block.
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return false
	}
	return reply[0] == 0x05 && reply[1] == 0x00
}

// TestShutdown checks the SIGTERM path against the real binary.
func TestShutdown(t *testing.T) {
	bin := buildBinary(t)

	dir := t.TempDir()
	socksPort := freePort(t)
	adminPort := freePort(t)
	hangAddr := startHangServer(t)
	cfg := writeShutdownConfig(t, dir, socksPort, adminPort, "http://"+hangAddr+"/gfwlist.txt")

	cmd := exec.Command(bin, "-conf", cfg)
	var logs strings.Builder
	cmd.Stdout = &logs
	cmd.Stderr = &logs
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting binary: %v", err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
		if t.Failed() {
			t.Logf("server output:\n%s", logs.String())
		}
	})

	base := fmt.Sprintf("http://127.0.0.1:%d", adminPort)
	waitReady(t, base, &logs)

	// Wait for the SOCKS5 listener to come up.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !socks5Greets(socksPort) {
		time.Sleep(100 * time.Millisecond)
	}
	if !socks5Greets(socksPort) {
		t.Fatalf("the SOCKS5 port never served a greeting\noutput:\n%s", logs.String())
	}

	// Start a GFW fetch so the admin server has an in-flight request and the
	// process stays alive across the shutdown. Without this the process
	// would exit immediately and the OS would release the port, which hides
	// the bug: the proxy has to stop serving while it is still alive.
	fetchDone := make(chan struct{})
	go func() {
		defer close(fetchDone)
		req, err := http.NewRequest(http.MethodPost, base+"/api/gfw/fetch", nil)
		if err != nil {
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}()
	time.Sleep(1500 * time.Millisecond) // let the fetch reach the hang server

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("sending SIGTERM: %v", err)
	}

	// The SOCKS5 port must stop serving immediately.
	servedAfter := false
	for i := 0; i < 60; i++ {
		if socks5Greets(socksPort) {
			servedAfter = true
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if servedAfter {
		t.Fatalf("the SOCKS5 port was still serving after SIGTERM\noutput:\n%s", logs.String())
	}

	// The process must still exit. The admin shutdown is bounded, so it
	// cannot wait forever on the in-flight fetch.
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case <-exited:
	case <-time.After(20 * time.Second):
		t.Fatalf("the process did not exit within 20s of SIGTERM\noutput:\n%s", logs.String())
	}

	// The fetch is still hanging; release it so the test does not leak it.
	_ = fetchDone
}
