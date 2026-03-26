package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/admin"
	"github.com/yourorg/proxy-pac/gfw"
	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/router"
	"github.com/yourorg/proxy-pac/status"
	"gopkg.in/yaml.v3"
)

type mockStatus struct{}

func (m *mockStatus) Status() status.ServerStatus {
	return status.ServerStatus{
		HTTPListen:   ":8080",
		SOCKS5Listen: ":1080",
		AdminListen:  "127.0.0.1:9090",
	}
}
func (m *mockStatus) Connections() []status.Connection { return nil }

func TestConfigAPI_IntegrationRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	pacPath := filepath.Join(tmpDir, "pac.js")

	// Write a minimal PAC file
	pacContent := gfw.GeneratePAC(nil, nil, gfw.FWMeta{})
	if err := os.WriteFile(pacPath, []byte(pacContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write initial config with HTTPS and TLS
	initial := fmt.Sprintf(`proxy:
  http_listen: ":8080"
  socks5_listen: ":1080"
  admin_listen: "127.0.0.1:9090"
  https_listen: ":8443"
  pac_path: "%s"
  default_external_proxy: ""
  external_proxies: []
  tls:
    certificate_file: "/old/cert.pem"
    key_file: "/old/key.pem"
    ca_file: ""
    client_auth: ""
`, pacPath)
	if err := os.WriteFile(configPath, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	// Create evaluator (minimal)
	ev, err := pac.NewEvaluator(pac.EvaluatorConfig{
		PACPath:        pacPath,
		LocalIP:        "127.0.0.1",
		DNSTimeout:     50 * time.Millisecond,
		EvalTimeout:    50 * time.Millisecond,
		EnableDNSCache: false,
		FallbackRoute:  pac.Route{Strategy: "DIRECT"},
	})
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer ev.Close()

	// Create router
	r, err := router.NewRouter(pac.RouterConfig{
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	// Create admin handler
	h := admin.NewHandler(
		ev,
		pacPath,
		nil, // fetcher
		"",  // userPath
		"",  // gfwPath
		configPath,
		&mockStatus{},
		r,
		"", // defaultExternalProxy
	)

	// Setup HTTP server
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// GET initial state
	resp, err := http.Get(srv.URL + "/api/config")
	if err != nil {
		t.Fatalf("GET config: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status: %d", resp.StatusCode)
	}
	var getBefore struct {
		HTTPSListen string `json:"https_listen"`
		TLS         struct {
			CertificateFile string `json:"certificate_file"`
			KeyFile         string `json:"key_file"`
		} `json:"tls"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&getBefore); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	resp.Body.Close()
	if getBefore.HTTPSListen != ":8443" {
		t.Errorf("initial https_listen = %q, want :8443", getBefore.HTTPSListen)
	}
	if getBefore.TLS.CertificateFile != "/old/cert.pem" {
		t.Errorf("initial cert = %q", getBefore.TLS.CertificateFile)
	}

	// PUT update
	update := `{
  "https_listen": ":9443",
  "tls": {
    "certificate_file": "/new/cert.pem",
    "key_file": "/new/key.pem",
    "ca_file": "/new/ca.pem",
    "client_auth": "require_verify"
  }
}`
	req, err := http.NewRequest(http.MethodPut, srv.URL+"/api/config", strings.NewReader(update))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT config: %v", err)
	}
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status: %d", putResp.StatusCode)
	}
	var putResult struct {
		RestartRequired bool `json:"restart_required"`
	}
	if err := json.NewDecoder(putResp.Body).Decode(&putResult); err != nil {
		t.Fatalf("decode PUT: %v", err)
	}
	putResp.Body.Close()
	if !putResult.RestartRequired {
		t.Error("restart_expected true after listener/TLS change")
	}

	// Verify config file on disk updated
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var fullCfg struct {
		Proxy struct {
			HTTPSListen string `yaml:"https_listen"`
			TLS         struct {
				CertificateFile string `yaml:"certificate_file"`
				KeyFile         string `yaml:"key_file"`
				CAFile          string `yaml:"ca_file"`
				ClientAuth      string `yaml:"client_auth"`
			} `yaml:"tls"`
		} `yaml:"proxy"`
	}
	if err := yaml.Unmarshal(data, &fullCfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if fullCfg.Proxy.HTTPSListen != ":9443" {
		t.Errorf("after PUT, HTTPSListen = %q, want :9443", fullCfg.Proxy.HTTPSListen)
	}
	if fullCfg.Proxy.TLS.CertificateFile != "/new/cert.pem" {
		t.Errorf("after PUT, cert = %q", fullCfg.Proxy.TLS.CertificateFile)
	}
	if fullCfg.Proxy.TLS.CAFile != "/new/ca.pem" {
		t.Errorf("after PUT, ca = %q", fullCfg.Proxy.TLS.CAFile)
	}

	// GET again to see updated values via handler
	resp2, err := http.Get(srv.URL + "/api/config")
	if err != nil {
		t.Fatalf("second GET: %v", err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("second GET status: %d", resp2.StatusCode)
	}
	var getAfter struct {
		HTTPSListen string `json:"https_listen"`
		TLS         struct {
			CertificateFile string `json:"certificate_file"`
			KeyFile         string `json:"key_file"`
			CAFile          string `json:"ca_file"`
		} `json:"tls"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&getAfter); err != nil {
		t.Fatalf("decode second GET: %v", err)
	}
	resp2.Body.Close()
	if getAfter.HTTPSListen != ":9443" {
		t.Errorf("after PUT, GET https_listen = %q, want :9443", getAfter.HTTPSListen)
	}
	if getAfter.TLS.CertificateFile != "/new/cert.pem" {
		t.Errorf("after PUT, GET cert = %q", getAfter.TLS.CertificateFile)
	}
}
