package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/router"
	"github.com/yourorg/proxy-pac/status"
	"gopkg.in/yaml.v3"
)

func kaHandler(t *testing.T, initial string) (*Handler, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := router.NewRouter(pac.RouterConfig{})
	if err != nil {
		t.Fatal(err)
	}
	return &Handler{
		statusProvider: &mockStatusProvider{serverStatus: status.ServerStatus{}},
		configPath:     path,
		router:         rt,
	}, path
}

func putConfig(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.handleConfig(w, httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body)))
	return w
}

func getConfig(t *testing.T, h *Handler) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	h.handleConfig(w, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/config = %d", w.Code)
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

const baseCfg = "proxy:\n  http_listen: \":8080\"\n  socks5_listen: \":1080\"\n"

func TestPutConfigSavesTCPKeepalive(t *testing.T) {
	h, path := kaHandler(t, baseCfg)

	w := putConfig(t, h, `{"tcp_keepalive":{"enabled":true,"idle":"45s","interval":"12s","count":4}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT = %d: %s", w.Code, w.Body.String())
	}

	data, _ := os.ReadFile(path)
	var cfg FullConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	ka := cfg.Proxy.TCPKeepAlive
	if !ka.Enabled || ka.Idle.String() != "45s" || ka.Interval.String() != "12s" || ka.Count != 4 {
		t.Errorf("persisted %+v, want enabled/45s/12s/4", ka)
	}

	// And it must read back through GET.
	got := getConfig(t, h)["tcp_keepalive"].(map[string]any)
	if got["enabled"] != true || got["idle"] != "45s" || got["count"].(float64) != 4 {
		t.Errorf("GET returned %+v", got)
	}
}

func TestPutConfigPartialKeepaliveKeepsOtherFields(t *testing.T) {
	h, path := kaHandler(t, baseCfg)
	putConfig(t, h, `{"tcp_keepalive":{"enabled":true,"idle":"45s","interval":"12s","count":4}}`)

	// Only toggle enabled: idle/interval/count must survive.
	w := putConfig(t, h, `{"tcp_keepalive":{"enabled":false}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT = %d", w.Code)
	}
	data, _ := os.ReadFile(path)
	var cfg FullConfig
	yaml.Unmarshal(data, &cfg)
	ka := cfg.Proxy.TCPKeepAlive
	if ka.Enabled {
		t.Error("enabled should be false")
	}
	if ka.Idle.String() != "45s" || ka.Interval.String() != "12s" || ka.Count != 4 {
		t.Errorf("other fields lost: %+v", ka)
	}
}

func TestPutConfigSavesSOCKS5Auth(t *testing.T) {
	h, path := kaHandler(t, baseCfg)

	w := putConfig(t, h, `{"socks5_auth":{"username":"alice","password":"s3cret"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT = %d: %s", w.Code, w.Body.String())
	}
	data, _ := os.ReadFile(path)
	var cfg FullConfig
	yaml.Unmarshal(data, &cfg)
	if cfg.Proxy.SOCKS5Auth == nil || cfg.Proxy.SOCKS5Auth.Username != "alice" || cfg.Proxy.SOCKS5Auth.Password != "s3cret" {
		t.Fatalf("auth not persisted: %+v", cfg.Proxy.SOCKS5Auth)
	}

	// GET must not return the password.
	got := getConfig(t, h)["socks5_auth"].(map[string]any)
	if got["username"] != "alice" {
		t.Errorf("username = %v", got["username"])
	}
	if got["has_password"] != true {
		t.Errorf("has_password = %v, want true", got["has_password"])
	}
	for k := range got {
		if strings.Contains(strings.ToLower(k), "password") && k != "has_password" {
			t.Errorf("GET exposes %q", k)
		}
	}
	if strings.Contains(got["username"].(string), "s3cret") {
		t.Error("password leaked in GET")
	}

	// Blank password keeps the stored one.
	putConfig(t, h, `{"socks5_auth":{"username":"bob"}}`)
	data, _ = os.ReadFile(path)
	yaml.Unmarshal(data, &cfg)
	if cfg.Proxy.SOCKS5Auth.Username != "bob" || cfg.Proxy.SOCKS5Auth.Password != "s3cret" {
		t.Errorf("blank password should be preserved, got %+v", cfg.Proxy.SOCKS5Auth)
	}

	// Empty object clears auth entirely. Re-read into a fresh value: yaml
	// unmarshalling into an already-populated struct does not reset fields
	// that are absent from the new document.
	putConfig(t, h, `{"socks5_auth":{"username":"","password":""}}`)
	data, _ = os.ReadFile(path)
	var cleared FullConfig
	yaml.Unmarshal(data, &cleared)
	if cleared.Proxy.SOCKS5Auth != nil {
		t.Errorf("auth should be cleared, got %+v", cleared.Proxy.SOCKS5Auth)
	}
	if strings.Contains(string(data), "socks5_auth") {
		t.Errorf("socks5_auth still present in config:\n%s", data)
	}
}

func TestPutConfigHandshakeTimeout(t *testing.T) {
	h, path := kaHandler(t, baseCfg)

	if w := putConfig(t, h, `{"socks5_handshake_timeout":"7s"}`); w.Code != http.StatusOK {
		t.Fatalf("PUT = %d: %s", w.Code, w.Body.String())
	}
	data, _ := os.ReadFile(path)
	var cfg FullConfig
	yaml.Unmarshal(data, &cfg)
	if cfg.Proxy.SOCKS5HandshakeTimeout.String() != "7s" {
		t.Errorf("timeout = %v, want 7s", cfg.Proxy.SOCKS5HandshakeTimeout)
	}
	if got := getConfig(t, h)["socks5_handshake_timeout"]; got != "7s" {
		t.Errorf("GET timeout = %v", got)
	}

	// Invalid durations are rejected and must not corrupt the file.
	for _, bad := range []string{`"nonsense"`, `"-5s"`, `"0s"`} {
		if w := putConfig(t, h, `{"socks5_handshake_timeout":`+bad+`}`); w.Code != http.StatusBadRequest {
			t.Errorf("PUT %s = %d, want 400", bad, w.Code)
		}
	}
	data, _ = os.ReadFile(path)
	yaml.Unmarshal(data, &cfg)
	if cfg.Proxy.SOCKS5HandshakeTimeout.String() != "7s" {
		t.Errorf("rejected write corrupted the value: %v", cfg.Proxy.SOCKS5HandshakeTimeout)
	}
}

func TestPutConfigRejectsBadKeepalive(t *testing.T) {
	h, path := kaHandler(t, baseCfg)
	for _, bad := range []string{
		`{"tcp_keepalive":{"enabled":true,"idle":"zzz"}}`,
		`{"tcp_keepalive":{"enabled":true,"interval":"-1s"}}`,
		`{"tcp_keepalive":{"enabled":true,"count":-3}}`,
	} {
		if w := putConfig(t, h, bad); w.Code != http.StatusBadRequest {
			t.Errorf("PUT %s = %d, want 400", bad, w.Code)
		}
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "tcp_keepalive") {
		t.Errorf("rejected writes still modified the file:\n%s", data)
	}
}

// A restart_required flag must be set for the settings read at startup.
func TestPutConfigFlagsRestart(t *testing.T) {
	h, _ := kaHandler(t, baseCfg)
	for _, body := range []string{
		`{"socks5_auth":{"username":"a","password":"b"}}`,
		`{"socks5_handshake_timeout":"7s"}`,
		`{"tcp_keepalive":{"enabled":true}}`,
	} {
		w := putConfig(t, h, body)
		var resp struct {
			RestartRequired bool `json:"restart_required"`
		}
		json.Unmarshal(w.Body.Bytes(), &resp)
		if !resp.RestartRequired {
			t.Errorf("%s did not set restart_required", body)
		}
	}
}
