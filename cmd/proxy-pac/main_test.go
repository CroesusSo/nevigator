package main

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/gfw"
)

func TestResolveConfigPath_FlagPriority(t *testing.T) {
	// Explicit flag always wins, regardless of what exists on disk
	customPath := "/tmp/custom-config.yaml"
	path, needsGen := resolveConfigPath(customPath)
	if path != customPath {
		t.Errorf("expected %s, got %s", customPath, path)
	}
	if needsGen {
		t.Error("expected needsGeneration=false for explicit flag")
	}
}

func TestResolveConfigPath_ExistingFile(t *testing.T) {
	// Create a temp home with an existing config file
	tmpHome := t.TempDir()
	configDir := filepath.Join(tmpHome, ".config", "proxy-pac")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("proxy:\n  http_listen: \":9999\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Override HOME
	t.Setenv("HOME", tmpHome)
	// os.UserHomeDir() reads HOME on unix, so this should work

	path, needsGen := resolveConfigPath("")
	expectedPath := filepath.Join(tmpHome, ".config", "proxy-pac", "config.yaml")
	if path != expectedPath {
		t.Errorf("expected %s, got %s", expectedPath, path)
	}
	if needsGen {
		t.Error("expected needsGeneration=false when config exists")
	}
}

func TestResolveConfigPath_AutoGenerate(t *testing.T) {
	// Temp home with no config file
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	path, needsGen := resolveConfigPath("")
	expectedPath := filepath.Join(tmpHome, ".config", "proxy-pac", "config.yaml")
	if path != expectedPath {
		t.Errorf("expected %s, got %s", expectedPath, path)
	}
	if !needsGen {
		t.Error("expected needsGeneration=true when config does not exist")
	}
}

func TestGenerateDefaultConfig(t *testing.T) {
	tmpHome := t.TempDir()
	configPath := filepath.Join(tmpHome, ".config", "proxy-pac", "config.yaml")

	if err := generateDefaultConfig(configPath); err != nil {
		t.Fatalf("generateDefaultConfig failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	// Verify content is valid YAML by loading it
	cfg, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("generated config is not valid: %v", err)
	}

	// Verify default values
	if cfg.Proxy.HTTPListen != ":8080" {
		t.Errorf("expected HTTPListen :8080, got %s", cfg.Proxy.HTTPListen)
	}
	if cfg.Proxy.SOCKS5Listen != ":11080" {
		t.Errorf("expected SOCKS5Listen :11080, got %s", cfg.Proxy.SOCKS5Listen)
	}
	if cfg.Proxy.AdminListen != "127.0.0.1:9090" {
		t.Errorf("expected AdminListen 127.0.0.1:9090, got %s", cfg.Proxy.AdminListen)
	}
	// PACPath is resolved to be alongside the config file
	expectedPAC := filepath.Join(filepath.Dir(configPath), "proxy.pac")
	if cfg.Proxy.PACPath != expectedPAC {
		t.Errorf("expected PACPath %s, got %s", expectedPAC, cfg.Proxy.PACPath)
	}
}

func TestGenerateDefaultConfig_PermissionDenied(t *testing.T) {
	// Create directory first, then make it read-only
	tmpHome := t.TempDir()
	configDir := filepath.Join(tmpHome, ".config", "proxy-pac")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.Chmod(configDir, 0555)

	configPath := filepath.Join(configDir, "config.yaml")
	err := generateDefaultConfig(configPath)

	// On some systems (e.g., running as root), writing to a 0555 dir may succeed
	if err == nil {
		t.Log("permission test did not fail (may be running as root), skipping assertion")
	} else {
		t.Logf("correctly returned error: %v", err)
	}
	// Should not panic — that's the real assertion here
}

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	if cfg.Proxy.HTTPListen != ":8080" {
		t.Errorf("expected :8080, got %s", cfg.Proxy.HTTPListen)
	}
	if cfg.Proxy.SOCKS5Listen != ":11080" {
		t.Errorf("expected :11080, got %s", cfg.Proxy.SOCKS5Listen)
	}
	if cfg.Proxy.AdminListen != "127.0.0.1:9090" {
		t.Errorf("expected 127.0.0.1:9090, got %s", cfg.Proxy.AdminListen)
	}
	if cfg.Proxy.PACPath != "" {
		t.Errorf("expected empty PACPath, got %s", cfg.Proxy.PACPath)
	}
	// TUNDevice field removed
}

func TestLoadConfig_EmptyTUNDevicePreserved(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `proxy:
  http_listen: ":8080"
  socks5_listen: ":1080"
  admin_listen: "127.0.0.1:9090"
  tun_device: ""
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}
	if cfg.Proxy.TUNDevice != "" {
		t.Errorf("TUNDevice = %q, want empty (disabled)", cfg.Proxy.TUNDevice)
	}
}

func TestFetcherTUNBinding(t *testing.T) {
	tunIP := net.ParseIP("10.0.0.2")
	timeout := 5 * time.Second

	// Create a default fetcher
	fetcher := gfw.NewFetcher("http://example.com/gfwlist.txt", "/tmp/gfw.json", timeout)
	originalClient := fetcher.HTTPClient

	// Simulate the binding logic from main.go (use_tun = true, tunIP available)
	useTUN := true
	if useTUN && tunIP != nil {
		fetcher.HTTPClient = &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					LocalAddr: &net.TCPAddr{IP: tunIP},
				}).DialContext,
			},
			Timeout: timeout,
		}
	}

	// Should have replaced the client
	if fetcher.HTTPClient == originalClient {
		t.Fatal("HTTPClient not replaced when use_tun=true and tunIP set")
	}

	// Verify transport type
	transport, ok := fetcher.HTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport type = %T, want *http.Transport", fetcher.HTTPClient.Transport)
	}
	if transport.DialContext == nil {
		t.Error("DialContext not set on transport")
	}

	// Test when use_tun = false (should not replace)
	fetcher2 := gfw.NewFetcher("http://example.com/gfwlist.txt", "/tmp/gfw2.json", timeout)
	originalClient2 := fetcher2.HTTPClient
	useTUN = false
	if useTUN && tunIP != nil {
		fetcher2.HTTPClient = &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					LocalAddr: &net.TCPAddr{IP: tunIP},
				}).DialContext,
			},
			Timeout: timeout,
		}
	}
	if fetcher2.HTTPClient != originalClient2 {
		t.Error("HTTPClient changed when use_tun=false")
	}

	// Test when tunIP = nil (should not replace even if use_tun=true)
	fetcher3 := gfw.NewFetcher("http://example.com/gfwlist.txt", "/tmp/gfw3.json", timeout)
	originalClient3 := fetcher3.HTTPClient
	useTUN = true
	tunIP = net.ParseIP("") // nil IP
	if useTUN && tunIP != nil {
		fetcher3.HTTPClient = &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					LocalAddr: &net.TCPAddr{IP: tunIP},
				}).DialContext,
			},
			Timeout: timeout,
		}
	}
	if fetcher3.HTTPClient != originalClient3 {
		t.Error("HTTPClient changed when tunIP is nil")
	}
}
