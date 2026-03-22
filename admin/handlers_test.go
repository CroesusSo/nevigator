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

	"github.com/yourorg/proxy-pac/gfw"
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

	// Check for data attributes in JS renderRules function
	if !strings.Contains(html, "data-id") || !strings.Contains(html, "data-route") || !strings.Contains(html, "data-match") {
		t.Errorf("HTML missing data-id, data-route, or data-match attributes")
	}

	// Check for GFW checkbox and section
	if !strings.Contains(html, `id="show-gfw"`) {
		t.Errorf("HTML missing GFW show checkbox")
	}
	if !strings.Contains(html, `class="gfw-section"`) {
		t.Errorf("HTML missing gfw-section container")
	}

	// Check for validation function
	if !strings.Contains(html, "function validatePattern") {
		t.Errorf("HTML missing validatePattern function")
	}

	// Check for tooltips map
	if !strings.Contains(html, "tooltips") || !strings.Contains(html, "domain_suffix") {
		t.Errorf("HTML missing tooltips definitions")
	}

	// Check for edit functions
	if !strings.Contains(html, "function startEdit") {
		t.Errorf("HTML missing startEdit function")
	}
	if !strings.Contains(html, "function saveEdit") {
		t.Errorf("HTML missing saveEdit function")
	}
	if !strings.Contains(html, "function cancelEdit") {
		t.Errorf("HTML missing cancelEdit function")
	}

	// Check for keyboard event handler
	if !strings.Contains(html, "handleEditKeyup") || !strings.Contains(html, "keyup") {
		t.Errorf("HTML missing keyboard event handling")
	}

	// Check for route badge CSS classes
	if !strings.Contains(html, ".badge-tun") || !strings.Contains(html, ".badge-direct") || !strings.Contains(html, ".badge-socks5") {
		t.Errorf("HTML missing route badge CSS classes")
	}

	// Check for match type badge CSS classes
	if !strings.Contains(html, ".badge-host_glob") || !strings.Contains(html, ".badge-url_glob") {
		t.Errorf("HTML missing match type badge CSS classes")
	}

	// Check for lazy rendering variables and functions
	if !strings.Contains(html, "gfwRowHeight") || !strings.Contains(html, "renderGFWSlice") {
		t.Errorf("HTML missing lazy rendering scaffolding")
	}

	// Check for evaluation order hint
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
			TUNDevice:    "tun0",
			TUNIP:        "10.0.0.2",
			TUNUp:        true,
			ActiveConns:  3,
			TotalConns:   42,
		},
	}
	h := &Handler{statusProvider: mp}

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
	if got.TUNDevice != "tun0" {
		t.Errorf("TUNDevice = %q, want %q", got.TUNDevice, "tun0")
	}
	if !got.TUNUp {
		t.Error("TUNUp = false, want true")
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
			{ID: "conn-1", ClientAddr: "10.0.0.1:5000", DestAddr: "example.com:443", Route: "TUN", Protocol: "http", StartedAt: now},
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


func TestHandlePutGFWConfig_ToggleDisabled(t *testing.T) {
	// Setup temp directory and files
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	userRulesPath := filepath.Join(dir, "user-rules.json")
	gfwRulesPath := filepath.Join(dir, "gfw-rules.json")
	pacPath := filepath.Join(dir, "proxy.pac")

	// Create initial config with GFW disabled: false
	cfgYAML := `proxy:
  http_listen: ":8080"
  socks5_listen: ":1080"
  admin_listen: "127.0.0.1:9090"
  pac_path: "` + pacPath + `"
  default_external_proxy: ""

pac:
  enable_url_glob: false

gfw:
  disabled: false
  source_url: "http://example.com/gfwlist.txt"
  user_rules_path: "` + userRulesPath + `"
  local_rules_path: "` + gfwRulesPath + `"
  fetch_on_startup: false
  update_interval: 24h
  http_timeout: 10s
`
	if err := os.WriteFile(cfgPath, []byte(cfgYAML), 0644); err != nil {
		t.Fatal(err)
	}
	// Create empty rule files
	os.WriteFile(userRulesPath, []byte("[]"), 0644)
	os.WriteFile(gfwRulesPath, []byte("[]"), 0644)
	// Create dummy PAC file
	os.WriteFile(pacPath, []byte(`function FindProxyForURL(url, host) { return "DIRECT"; }`), 0644)

	// Load full config to get GFW config for fetcher
	data, err := os.ReadFile(cfgPath)
	if err != nil { t.Fatal(err) }
	var full FullConfig
	if err := yaml.Unmarshal(data, &full); err != nil { t.Fatal(err) }

	// Create real fetcher
	fetcher, err := gfw.NewFetcher(full.GFW)
	if err != nil { t.Fatal(err) }
	fetcher.SetDisabled(full.GFW.Disabled)

	// Construct handler with minimal fields
	h := &Handler{
		configPath:           cfgPath,
		fetcher:              fetcher,
		userPath:             full.GFW.UserRulesPath,
		gfwPath:              full.GFW.LocalRulesPath,
		pacPath:              full.Proxy.PACPath,
		defaultExternalProxy: full.Proxy.DefaultExternalProxy,
	}

	// Send PUT request to disable GFW
	reqBody := `{"disabled": true}`
	req := httptest.NewRequest(http.MethodPut, "/api/gfw/config", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.handleGFWConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	// Verify config file updated
	data, err = os.ReadFile(cfgPath)
	if err != nil { t.Fatal(err) }
	var updated FullConfig
	if err := yaml.Unmarshal(data, &updated); err != nil {
		t.Fatalf("parse updated config: %v", err)
	}
	if updated.GFW.Disabled != true {
		t.Errorf("GFW.Disabled = %v, want true", updated.GFW.Disabled)
	}

	// Verify fetcher state updated
	if !fetcher.IsDisabled() {
		t.Error("fetcher.IsDisabled() = false, want true")
	}
}
