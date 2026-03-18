package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/yourorg/proxy-pac/status"
)

// writeTempConfig creates a temporary config file with the given content and returns its path.
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHandleGetConfig_IncludesHTTPSAndTLS(t *testing.T) {
	cfgContent := `proxy:
  https_listen: ":8443"
  tls:
    certificate_file: "/cert.pem"
    key_file: "/key.pem"
    ca_file: "/ca.pem"
    client_auth: "require_verify"
`
	cfgPath := writeTempConfig(t, cfgContent)

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
	}

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()
	h.handleConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		HTTPSListen string `json:"https_listen"`
		TLS         struct {
			CertificateFile string `json:"certificate_file"`
			KeyFile         string `json:"key_file"`
			CAFile          string `json:"ca_file"`
			ClientAuth      string `json:"client_auth"`
		} `json:"tls"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.HTTPSListen != ":8443" {
		t.Errorf("HTTPSListen = %q, want %q", resp.HTTPSListen, ":8443")
	}
	if resp.TLS.CertificateFile != "/cert.pem" {
		t.Errorf("TLS.CertificateFile = %q, want %q", resp.TLS.CertificateFile, "/cert.pem")
	}
	if resp.TLS.KeyFile != "/key.pem" {
		t.Errorf("TLS.KeyFile = %q, want %q", resp.TLS.KeyFile, "/key.pem")
	}
	if resp.TLS.CAFile != "/ca.pem" {
		t.Errorf("TLS.CAFile = %q, want %q", resp.TLS.CAFile, "/ca.pem")
	}
	if resp.TLS.ClientAuth != "require_verify" {
		t.Errorf("TLS.ClientAuth = %q, want %q", resp.TLS.ClientAuth, "require_verify")
	}
}

func TestHandleGetConfig_WithoutHTTPSAndTLS(t *testing.T) {
	cfgContent := `proxy:
  http_listen: ":8080"
  pac_path: "/etc/proxy.pac"
`
	cfgPath := writeTempConfig(t, cfgContent)

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
	}

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()
	h.handleConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		HTTPSListen string `json:"https_listen"`
		TLS         struct {
			CertificateFile string `json:"certificate_file"`
			KeyFile         string `json:"key_file"`
		} `json:"tls"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.HTTPSListen != "" {
		t.Errorf("HTTPSListen = %q, want empty", resp.HTTPSListen)
	}
	if resp.TLS.CertificateFile != "" {
		t.Errorf("TLS.CertificateFile = %q, want empty", resp.TLS.CertificateFile)
	}
}

func TestHandlePutConfig_UpdatesHTTPSAndTLS(t *testing.T) {
	initial := `proxy:
  https_listen: ":8443"
  tls:
    certificate_file: "/old/cert.pem"
    key_file: "/old/key.pem"
    ca_file: ""
    client_auth: ""
`
	cfgPath := writeTempConfig(t, initial)
	h := &Handler{configPath: cfgPath}

	payload := map[string]interface{}{
		"https_listen": ":9443",
		"tls": map[string]interface{}{
			"certificate_file": "/new/cert.pem",
			"key_file":         "/new/key.pem",
			"ca_file":          "/new/ca.pem",
			"client_auth":      "require_verify",
		},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if restart, ok := resp["restart_required"].(bool); !ok || !restart {
		t.Errorf("restart_required = %v, want true", resp["restart_required"])
	}

	// Verify config file updated
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg FullConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if cfg.Proxy.HTTPSListen != ":9443" {
		t.Errorf("HTTPSListen = %q, want :9443", cfg.Proxy.HTTPSListen)
	}
	if cfg.Proxy.TLS.CertificateFile != "/new/cert.pem" {
		t.Errorf("TLS.CertificateFile = %q, want /new/cert.pem", cfg.Proxy.TLS.CertificateFile)
	}
	if cfg.Proxy.TLS.KeyFile != "/new/key.pem" {
		t.Errorf("TLS.KeyFile = %q, want /new/key.pem", cfg.Proxy.TLS.KeyFile)
	}
	if cfg.Proxy.TLS.CAFile != "/new/ca.pem" {
		t.Errorf("TLS.CAFile = %q, want /new/ca.pem", cfg.Proxy.TLS.CAFile)
	}
	if cfg.Proxy.TLS.ClientAuth != "require_verify" {
		t.Errorf("TLS.ClientAuth = %q, want require_verify", cfg.Proxy.TLS.ClientAuth)
	}
}

func TestHandlePutConfig_PreservesOtherFields(t *testing.T) {
	initial := `proxy:
  http_listen: ":8080"
  socks5_listen: ":1080"
  admin_listen: "127.0.0.1:9090"
  https_listen: ":8443"
  pac_path: "/old/pac.pac"
  tun_device: "tun0"
  default_external_proxy: ""
  external_proxies: []
  tls:
    certificate_file: "/cert.pem"
    key_file: "/key.pem"
    ca_file: ""
    client_auth: ""
`
	cfgPath := writeTempConfig(t, initial)
	h := &Handler{configPath: cfgPath}

	payload := map[string]interface{}{
		"pac_path": "/new/pac.pac",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	// restart_required false for pac_path change
	if restart, _ := resp["restart_required"].(bool); restart {
		t.Error("restart_expected false for pac_path update")
	}

	data, _ := os.ReadFile(cfgPath)
	var cfg FullConfig
	yaml.Unmarshal(data, &cfg)
	if cfg.Proxy.PACPath != "/new/pac.pac" {
		t.Errorf("PACPath not updated: got %q", cfg.Proxy.PACPath)
	}
	if cfg.Proxy.HTTPSListen != ":8443" {
		t.Errorf("HTTPSListen changed unexpectedly: %q", cfg.Proxy.HTTPSListen)
	}
	if cfg.Proxy.TLS.CertificateFile != "/cert.pem" {
		t.Errorf("TLS.CertificateFile changed: %q", cfg.Proxy.TLS.CertificateFile)
	}
}

func TestHandlePutConfig_BackwardCompatibility(t *testing.T) {
	// Config without https_listen and tls
	initial := `proxy:
  http_listen: ":8080"
  pac_path: "/old.pac"
`
	cfgPath := writeTempConfig(t, initial)
	h := &Handler{configPath: cfgPath}

	payload := map[string]interface{}{
		"http_listen": ":9090",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	data, _ := os.ReadFile(cfgPath)
	var cfg FullConfig
	yaml.Unmarshal(data, &cfg)
	// The https_listen and tls fields should remain zero (empty)
	if cfg.Proxy.HTTPSListen != "" {
		t.Errorf("HTTPSListen unexpectedly set to %q", cfg.Proxy.HTTPSListen)
	}
	if cfg.Proxy.TLS.CertificateFile != "" {
		t.Errorf("TLS.CertificateFile unexpectedly set to %q", cfg.Proxy.TLS.CertificateFile)
	}
	if cfg.Proxy.HTTPListen != ":9090" {
		t.Errorf("HTTPListen not updated: %q", cfg.Proxy.HTTPListen)
	}
}

func TestHandlePutConfig_TLSOnlyPartialUpdate(t *testing.T) {
	initial := `proxy:
  https_listen: ":8443"
  tls:
    certificate_file: "/old/cert.pem"
    key_file: "/old/key.pem"
    ca_file: "/old/ca.pem"
    client_auth: "none"
`
	cfgPath := writeTempConfig(t, initial)
	h := &Handler{configPath: cfgPath}

	payload := map[string]interface{}{
		"tls": map[string]interface{}{
			"certificate_file": "/new/cert.pem",
		},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if restart, _ := resp["restart_required"].(bool); !restart {
		t.Error("restart_required should be true for TLS update")
	}

	data, _ := os.ReadFile(cfgPath)
	var cfg FullConfig
	yaml.Unmarshal(data, &cfg)
	if cfg.Proxy.TLS.CertificateFile != "/new/cert.pem" {
		t.Errorf("certificate not updated")
	}
	if cfg.Proxy.TLS.KeyFile != "/old/key.pem" {
		t.Errorf("key_file should remain unchanged: %q", cfg.Proxy.TLS.KeyFile)
	}
	if cfg.Proxy.TLS.CAFile != "/old/ca.pem" {
		t.Errorf("ca_file should remain unchanged: %q", cfg.Proxy.TLS.CAFile)
	}
	if cfg.Proxy.TLS.ClientAuth != "none" {
		t.Errorf("client_auth should remain unchanged: %q", cfg.Proxy.TLS.ClientAuth)
	}
}

func TestHandlePutConfig_InvalidJSON(t *testing.T) {
	cfgPath := writeTempConfig(t, `proxy:{}`)
	// Need a statusProvider for GET path? Not used in PUT directly, but handleConfig may not require it for invalid JSON? Actually handlePutConfig does not use statusProvider at all. So we can pass nil.
	h := &Handler{configPath: cfgPath}

	req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(`{invalid}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandlePutConfig_RestartRequiredOnlyForListenersAndTLS(t *testing.T) {
	initial := `proxy:
  http_listen: ":8080"
  https_listen: ":8443"
  tls:
    certificate_file: "/cert.pem"
    key_file: "/key.pem"
    ca_file: ""
    client_auth: ""
`
	cfgPath := writeTempConfig(t, initial)
	h := &Handler{configPath: cfgPath}

	// Update only tun_device (non-restart-field)
	payload := map[string]interface{}{
		"tun_device": "tun1",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if restart, _ := resp["restart_required"].(bool); restart {
		t.Error("restart_required should be false for tun_device change")
	}

	// Verify tun_device updated
	data, _ := os.ReadFile(cfgPath)
	var cfg FullConfig
	yaml.Unmarshal(data, &cfg)
	if cfg.Proxy.TUNDevice != "tun1" {
		t.Errorf("TUNDevice not updated: %q", cfg.Proxy.TUNDevice)
	}
	// Ensure https_listen unchanged
	if cfg.Proxy.HTTPSListen != ":8443" {
		t.Errorf("HTTPSListen changed: %q", cfg.Proxy.HTTPSListen)
	}
}
