package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/router"
	"github.com/yourorg/proxy-pac/status"
)

func TestUIHasEnhancedFeatures(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.handleUI(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	html := w.Body.String()

	// Check that HTML references external CSS and JS files (separation of concerns)
	if !strings.Contains(html, `<link rel="stylesheet" href="/static/style.css">`) {
		t.Errorf("HTML missing link to external CSS file")
	}
	if !strings.Contains(html, `<script src="/static/script.js"></script>`) {
		t.Errorf("HTML missing script tag for external JS file")
	}

	// Check for essential HTML structure and IDs (content still present)
	if !strings.Contains(html, `id="show-gfw"`) {
		t.Errorf("HTML missing GFW show checkbox")
	}
	if !strings.Contains(html, `class="gfw-section"`) {
		t.Errorf("HTML missing gfw-section container")
	}
	if !strings.Contains(html, `id="rules-body"`) {
		t.Errorf("HTML missing rules table body")
	}
	if !strings.Contains(html, `id="new-match"`) {
		t.Errorf("HTML missing new-match select")
	}

	// Check for evaluation order hint (text content in HTML)
	if !strings.Contains(html, "Rules are evaluated top-to-bottom") {
		t.Errorf("HTML missing evaluation order hint")
	}

	// Check for HTTPS proxy field
	if !strings.Contains(html, `id="https-listen"`) {
		t.Errorf("HTML missing https-listen input")
	}
	// Check for TLS fields
	if !strings.Contains(html, `id="tls-certificate"`) {
		t.Errorf("HTML missing TLS certificate input")
	}
	if !strings.Contains(html, `id="tls-key"`) {
		t.Errorf("HTML missing TLS key input")
	}
	if !strings.Contains(html, `id="tls-ca"`) {
		t.Errorf("HTML missing TLS CA input")
	}
	if !strings.Contains(html, `id="tls-client-auth"`) {
		t.Errorf("HTML missing TLS client auth select")
	}
	// Check for TLS card
	if !strings.Contains(html, `<h3>TLS Settings</h3>`) {
		t.Errorf("HTML missing TLS Settings card")
	}
}

func TestExtractHost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com/path?query=1", "example.com"},
		{"http://example.com", "example.com"},
		{"example.com", "example.com"},
		{"example.com:8080", "example.com"},
		{"sub.domain.example.co.uk", "sub.domain.example.co.uk"},
		{"http://192.168.1.1:3000/api", "192.168.1.1"},
		{"192.168.1.1:8080", "192.168.1.1"},
		{"https://user:pass@example.com/path", "example.com"},
	}
	for _, tt := range tests {
		got := extractHost(tt.input)
		if got != tt.want {
			t.Errorf("extractHost(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

type mockStatusProvider struct {
	serverStatus status.ServerStatus
	conns        []status.Connection
}

func (m *mockStatusProvider) Status() status.ServerStatus      { return m.serverStatus }
func (m *mockStatusProvider) Connections() []status.Connection { return m.conns }

func TestHandleStatusAPI(t *testing.T) {
	mp := &mockStatusProvider{
		serverStatus: status.ServerStatus{
			HTTPListen:   ":8080",
			SOCKS5Listen: ":1080",
			AdminListen:  "127.0.0.1:9090",
			ActiveConns:  3,
			TotalConns:   42,
		},
	}
	h := &Handler{statusProvider: mp, router: &router.Router{}}

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()
	h.handleStatusAPI(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var got status.ServerStatus
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.HTTPListen != ":8080" {
		t.Errorf("HTTPListen = %q, want %q", got.HTTPListen, ":8080")
	}
	if got.ActiveConns != 3 {
		t.Errorf("ActiveConns = %d, want 3", got.ActiveConns)
	}
	if got.TotalConns != 42 {
		t.Errorf("TotalConns = %d, want 42", got.TotalConns)
	}
}

func TestHandleStatusAPIMethodNotAllowed(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/api/status", nil)
	w := httptest.NewRecorder()
	h.handleStatusAPI(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleConnections(t *testing.T) {
	now := time.Now()
	mp := &mockStatusProvider{
		conns: []status.Connection{
			{ID: "conn-1", ClientAddr: "10.0.0.1:5000", DestAddr: "example.com:443", Route: "SOCKS5", Protocol: "http", StartedAt: now},
			{ID: "conn-2", ClientAddr: "10.0.0.1:5001", DestAddr: "local.dev:80", Route: "DIRECT", Protocol: "socks5", StartedAt: now},
		},
	}
	h := &Handler{statusProvider: mp}

	req := httptest.NewRequest(http.MethodGet, "/api/connections", nil)
	w := httptest.NewRecorder()
	h.handleConnections(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var got []status.Connection
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != "conn-1" {
		t.Errorf("conn[0].ID = %q, want %q", got[0].ID, "conn-1")
	}
	if got[1].Route != "DIRECT" {
		t.Errorf("conn[1].Route = %q, want %q", got[1].Route, "DIRECT")
	}
}

func TestHandleConnectionsEmpty(t *testing.T) {
	mp := &mockStatusProvider{conns: []status.Connection{}}
	h := &Handler{statusProvider: mp}

	req := httptest.NewRequest(http.MethodGet, "/api/connections", nil)
	w := httptest.NewRecorder()
	h.handleConnections(w, req)

	var got []status.Connection
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestHandleConnectionsNilProvider(t *testing.T) {
	h := &Handler{statusProvider: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/connections", nil)
	w := httptest.NewRecorder()
	h.handleConnections(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var got []status.Connection
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestValidateProxyAddress(t *testing.T) {
	ourHTTP := ":8080"
	ourSOCKS5 := ":1080"
	ourAdmin := ":9090"

	cases := []struct {
		addr    string
		wantErr bool
	}{
		{"127.0.0.1:8080", true},    // loopback on HTTP port -> error
		{"localhost:1080", true},    // loopback on SOCKS5 port -> error
		{"[::1]:9090", true},        // IPv6 loopback on admin port -> error
		{"127.0.0.1:9999", false},   // loopback but not a listener port -> ok
		{"localhost:1234", false},   // loopback other port -> ok
		{"example.com:1080", false}, // non-loopback -> ok
	}
	for _, c := range cases {
		err := validateProxyAddress(c.addr, ourHTTP, ourSOCKS5, ourAdmin)
		if c.wantErr {
			if err == nil {
				t.Errorf("validateProxyAddress(%s) expected error, got nil", c.addr)
			}
		} else {
			if err != nil {
				t.Errorf("validateProxyAddress(%s) expected no error, got %v", c.addr, err)
			}
		}
	}
}

// New tests for static asset handling
func TestHandleStatic_CSS(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/static/style.css", nil)
	w := httptest.NewRecorder()
	h.handleStatic(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	ctype := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ctype, "text/css") {
		t.Errorf("Content-Type = %q, want text/css", ctype)
	}

	body := w.Body.String()
	if body == "" {
		t.Error("CSS body is empty")
	}
	if !strings.Contains(body, ".badge") {
		t.Error("CSS missing .badge class")
	}
}

func TestHandleStatic_JS(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/static/script.js", nil)
	w := httptest.NewRecorder()
	h.handleStatic(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	ctype := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ctype, "application/javascript") {
		t.Errorf("Content-Type = %q, want application/javascript", ctype)
	}

	body := w.Body.String()
	if body == "" {
		t.Error("JS body is empty")
	}
	if !strings.Contains(body, "function loadStatus") {
		t.Error("JS missing loadStatus function")
	}
}

func TestHandleStatic_NotFound(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/static/nonexistent.txt", nil)
	w := httptest.NewRecorder()
	h.handleStatic(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleStatic_PathTraversal(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/static/../handlers.go", nil)
	w := httptest.NewRecorder()
	h.handleStatic(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleCreateProxy_AppliesDefaultKeepaliveInterval(t *testing.T) {
	// Prepare a temporary config with no external proxies
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	initialCfg := `
proxy:
  external_proxies: []
`
	if err := os.WriteFile(cfgPath, []byte(initialCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	mp := &mockStatusProvider{
		serverStatus: status.ServerStatus{
			HTTPListen:   ":8080",
			SOCKS5Listen: ":1080",
			AdminListen:  "127.0.0.1:9090",
		},
	}
	h := &Handler{
		statusProvider: mp,
		configPath:     cfgPath,
		router:         &router.Router{},
	}

	// POST JSON with keepalive_enabled true, but no keepalive_interval
	body := `{
		"id": "ssh-test",
		"address": "user@example.com",
		"kind": "ssh",
		"ssh_port": 22,
		"auth_method": "password",
		"password": "secret",
		"keepalive_enabled": true
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/proxies", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleProxies(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusCreated)
	}

	// Verify the config file was updated with default 30s interval
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var full struct {
		Proxy struct {
			ExternalProxies []pac.ExternalProxy `yaml:"external_proxies"`
		} `yaml:"proxy"`
	}
	if err := yaml.Unmarshal(data, &full); err != nil {
		t.Fatal(err)
	}
	if len(full.Proxy.ExternalProxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(full.Proxy.ExternalProxies))
	}
	p := full.Proxy.ExternalProxies[0]
	if !p.Keepalive.Enabled {
		t.Error("keepalive enabled = false, want true")
	}
	if p.Keepalive.Interval != 30*time.Second {
		t.Errorf("keepalive interval = %v, want 30s", p.Keepalive.Interval)
	}

	// Verify response encodes interval as "30s" (flat fields)
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if enabled, ok := resp["keepalive_enabled"].(bool); !ok || !enabled {
		t.Error("response keepalive_enabled missing or false")
	}
	if interval, ok := resp["keepalive_interval"].(string); !ok || interval != "30s" {
		t.Errorf("response keepalive_interval = %v, want \"30s\"", resp["keepalive_interval"])
	}
}

func TestHandleGetProxyByID_KeepaliveIntervalAsString(t *testing.T) {
	// Setup temp config with one SSH proxy having keepalive interval 45s
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfgContent := `
proxy:
  external_proxies:
    - id: "ssh-keepalive-test"
      kind: "ssh"
      address: "user@example.com"
      ssh_port: 22
      auth_method: "password"
      password: "secret"
      keepalive:
        enabled: true
        interval: 45s
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatal(err)
	}

	mp := &mockStatusProvider{
		serverStatus: status.ServerStatus{
			HTTPListen:   ":8080",
			SOCKS5Listen: ":1080",
			AdminListen:  "127.0.0.1:9090",
		},
	}
	h := &Handler{
		statusProvider: mp,
		configPath:     cfgPath,
		router:         &router.Router{},
	}

	// GET /api/proxies/ssh-keepalive-test
	req := httptest.NewRequest(http.MethodGet, "/api/proxies/ssh-keepalive-test", nil)
	w := httptest.NewRecorder()
	h.handleProxyByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	// Check keepalive is an object with enabled and interval as string
	if keepalive, ok := resp["keepalive"].(map[string]interface{}); ok {
		if enabled, ok := keepalive["enabled"].(bool); !ok || !enabled {
			t.Error("keepalive.enabled missing or false")
		}
		if interval, ok := keepalive["interval"].(string); !ok || interval != "45s" {
			t.Errorf("keepalive.interval = %v, want \"45s\"", keepalive["interval"])
		}
	} else {
		t.Error("response missing keepalive field or not an object")
	}
}
