// Smoke test: builds the real binary, starts it against a generated config and
// exercises the admin API end to end.
//
// Unit tests construct handlers directly, so they cannot catch wiring mistakes
// in cmd/proxy-pac — a field that exists on the struct but is never passed to
// proxy.NewServer still passes every test in this repository. This starts the
// actual process and checks what it serves.
//
// Run with: make smoke
// Package smoke holds the end-to-end check, kept out of the main package so it
// cannot be affected by its TestMain, which exits early unless
// RUN_SSH_TESTS=1.
package smoke

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// freePort asks the kernel for an unused port and releases it immediately.
// Racy in theory, fine in practice for a single local smoke run.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving a port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func writeConfig(t *testing.T, dir string, httpPort, socksPort, adminPort int) string {
	t.Helper()
	cfg := fmt.Sprintf(`proxy:
  http_listen: "127.0.0.1:%d"
  socks5_listen: "127.0.0.1:%d"
  admin_listen: "127.0.0.1:%d"
  pac_path: "%s"
  socks5_handshake_timeout: 7s
  tcp_keepalive:
    enabled: true
    idle: 15s
    interval: 5s
    count: 3
pac:
  eval_timeout: 1s
gfw:
  disabled: true
  fetch_on_startup: false
`, httpPort, socksPort, adminPort, filepath.Join(dir, "proxy.pac"))

	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	return path
}

// buildBinary compiles the command under test into a temp dir and returns its path.
func buildBinary(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "proxy-pac")
	cmd := exec.Command("go", "build", "-o", out, "github.com/yourorg/proxy-pac/cmd/proxy-pac")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, b)
	}
	return out
}

// TestSmoke starts the built binary and checks the endpoints the admin UI uses.
func TestSmoke(t *testing.T) {
	bin := buildBinary(t)

	dir := t.TempDir()
	httpPort := freePort(t)
	socksPort := freePort(t)
	adminPort := freePort(t)
	cfg := writeConfig(t, dir, httpPort, socksPort, adminPort)

	// Start the process. Output is captured so a startup failure explains itself.
	cmd := exec.Command(bin, "-conf", cfg)
	logs := newLogWatcher()
	cmd.Stdout = logs
	cmd.Stderr = logs
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
	waitReady(t, base, logs)

	t.Run("GET /api/config exposes the new settings", func(t *testing.T) {
		var cfgResp struct {
			HTTPListen             string `json:"http_listen"`
			SOCKS5Listen           string `json:"socks5_listen"`
			AdminListen            string `json:"admin_listen"`
			SOCKS5HandshakeTimeout string `json:"socks5_handshake_timeout"`
			SOCKS5Auth             struct {
				Username  string `json:"username"`
				HasPasswd bool   `json:"has_password"`
			} `json:"socks5_auth"`
			TCPKeepAlive struct {
				Enabled  bool   `json:"enabled"`
				Idle     string `json:"idle"`
				Interval string `json:"interval"`
				Count    int    `json:"count"`
			} `json:"tcp_keepalive"`
		}
		getJSON(t, base+"/api/config", &cfgResp)

		if cfgResp.HTTPListen == "" || cfgResp.SOCKS5Listen == "" || cfgResp.AdminListen == "" {
			t.Errorf("listeners missing: %+v", cfgResp)
		}
		// Values must come back as the file has them, not as Go defaults.
		if cfgResp.SOCKS5HandshakeTimeout != "7s" {
			t.Errorf("socks5_handshake_timeout = %q, want 7s", cfgResp.SOCKS5HandshakeTimeout)
		}
		if !cfgResp.TCPKeepAlive.Enabled || cfgResp.TCPKeepAlive.Idle != "15s" ||
			cfgResp.TCPKeepAlive.Interval != "5s" || cfgResp.TCPKeepAlive.Count != 3 {
			t.Errorf("tcp_keepalive = %+v, want enabled 15s/5s/3", cfgResp.TCPKeepAlive)
		}
	})

	t.Run("empty collections are arrays not null", func(t *testing.T) {
		// A nil slice marshals to null, which breaks clients that iterate.
		for _, path := range []string{"/api/connections", "/api/rules", "/api/proxies"} {
			body := getRaw(t, base+path)
			if strings.TrimSpace(body) == "null" {
				t.Errorf("%s returned null, want []", path)
				continue
			}
			var v []any
			if err := json.Unmarshal([]byte(body), &v); err != nil {
				t.Errorf("%s did not return an array: %s", path, body)
			}
		}
	})

	t.Run("GET /api/status reports the listeners", func(t *testing.T) {
		var st map[string]any
		getJSON(t, base+"/api/status", &st)
		if st["httpListen"] != fmt.Sprintf("127.0.0.1:%d", httpPort) {
			t.Errorf("httpListen = %v", st["httpListen"])
		}
	})

	t.Run("PUT /api/config round-trips a setting", func(t *testing.T) {
		body := `{"tcp_keepalive":{"enabled":true,"idle":"45s","interval":"12s","count":4}}`
		resp := putJSON(t, base+"/api/config", body)
		var put struct {
			Status          string `json:"status"`
			RestartRequired bool   `json:"restart_required"`
		}
		if err := json.Unmarshal([]byte(resp), &put); err != nil {
			t.Fatalf("PUT returned %s", resp)
		}
		if put.Status != "ok" || !put.RestartRequired {
			t.Errorf("PUT /api/config = %+v, want status ok and restart_required", put)
		}

		// It must be readable back, and persisted to disk.
		var cfgResp struct {
			TCPKeepAlive struct {
				Enabled  bool   `json:"enabled"`
				Idle     string `json:"idle"`
				Interval string `json:"interval"`
				Count    int    `json:"count"`
			} `json:"tcp_keepalive"`
		}
		getJSON(t, base+"/api/config", &cfgResp)
		if cfgResp.TCPKeepAlive.Idle != "45s" || cfgResp.TCPKeepAlive.Count != 4 {
			t.Errorf("after PUT, tcp_keepalive = %+v", cfgResp.TCPKeepAlive)
		}
		onDisk, err := os.ReadFile(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(onDisk), "45s") {
			t.Errorf("update not persisted to disk:\n%s", onDisk)
		}
	})

	t.Run("PUT rejects an invalid duration", func(t *testing.T) {
		code, _ := request(t, http.MethodPut, base+"/api/config", `{"socks5_handshake_timeout":"not-a-duration"}`)
		if code != http.StatusBadRequest {
			t.Errorf("invalid duration = %d, want 400", code)
		}
	})

	t.Run("the SOCKS5 listener answers a greeting", func(t *testing.T) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", socksPort), 2*time.Second)
		if err != nil {
			t.Fatalf("dial socks5: %v", err)
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(2 * time.Second))
		// VER=5, NMETHODS=1, METHODS=[no auth]
		if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
			t.Fatalf("writing greeting: %v", err)
		}
		reply := make([]byte, 2)
		if _, err := io.ReadFull(conn, reply); err != nil {
			t.Fatalf("reading method reply: %v", err)
		}
		if reply[0] != 0x05 || reply[1] != 0x00 {
			t.Errorf("method reply = %v, want [5 0]", reply)
		}
	})

	t.Run("the HTTP proxy CONNECTs through to a local target", func(t *testing.T) {
		// A local echo target keeps the test offline: the proxy must dial out
		// and relay, which is what proves routing ran end to end.
		target, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("starting target: %v", err)
		}
		defer target.Close()
		go func() {
			for {
				c, err := target.Accept()
				if err != nil {
					return
				}
				go func(c net.Conn) {
					defer c.Close()
					c.Write([]byte("TARGET"))
					c.Read(make([]byte, 4))
				}(c)
			}
		}()

		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", httpPort), 2*time.Second)
		if err != nil {
			t.Fatalf("dial http proxy: %v", err)
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))

		fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target.Addr(), target.Addr())
		buf := make([]byte, 256)
		n, err := conn.Read(buf)
		if err != nil && n == 0 {
			t.Fatalf("no response to CONNECT: %v", err)
		}
		if !strings.Contains(string(buf[:n]), "200") {
			t.Fatalf("CONNECT not established: %q", string(buf[:n]))
		}

		// Traffic must then flow through the tunnel to the target.
		if _, err := conn.Write([]byte("ping")); err != nil {
			t.Fatalf("write through tunnel: %v", err)
		}
		got := make([]byte, 6)
		if _, err := io.ReadFull(conn, got); err != nil {
			t.Fatalf("read through tunnel: %v", err)
		}
		if string(got) != "TARGET" {
			t.Errorf("relayed %q, want TARGET", got)
		}
	})
}

// waitReady polls the admin endpoint until the server responds or we give up.
func waitReady(t *testing.T, base string, logs *logWatcher) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/api/status")
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("server did not become ready in 15s\noutput:\n%s", logs.String())
}

func request(t *testing.T, method, url, body string) (int, string) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func getRaw(t *testing.T, url string) string {
	t.Helper()
	code, body := request(t, http.MethodGet, url, "")
	if code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", url, code, body)
	}
	return body
}

func getJSON(t *testing.T, url string, v any) {
	t.Helper()
	body := getRaw(t, url)
	if err := json.Unmarshal([]byte(body), v); err != nil {
		t.Fatalf("GET %s returned unparseable body: %v\n%s", url, err, body)
	}
}

func putJSON(t *testing.T, url, body string) string {
	t.Helper()
	code, resp := request(t, http.MethodPut, url, body)
	if code != http.StatusOK {
		t.Fatalf("PUT %s = %d: %s", url, code, resp)
	}
	return resp
}
