package pac

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/gfw"
)

func TestEvaluator(t *testing.T) {
	// Create a simple PAC that routes example.com to SOCKS5
	rules := []gfw.Rule{
		{ID: "test", Source: "test", Enabled: true, Match: "exact", Value: "example.com", Action: "PROXY", Route: "SOCKS5 127.0.0.1:9050"},
	}
	pac := gfw.GeneratePAC(rules, nil, gfw.FWMeta{})
	tmp := t.TempDir() + "/test.pac"
	if err := os.WriteFile(tmp, []byte(pac), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := EvaluatorConfig{
		PACPath:        tmp,
		LocalIP:        "10.0.0.2",
		DNSTimeout:     100 * time.Millisecond,
		EvalTimeout:    100 * time.Millisecond,
		EnableDNSCache: false,
		FallbackRoute:  Route{Strategy: "DIRECT"},
	}
	ev, err := NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer ev.Close()

	route := ev.Evaluate("example.com")
	if route.Strategy != "SOCKS5" {
		t.Errorf("Expected SOCKS5, got %v", route)
	}
}

func TestEvaluatorSyntaxError(t *testing.T) {
	// Invalid JSON in __PAC_RULES__
	pac := `var __PAC_RULES__ = {invalid json}; function FindProxyForURL(url, host) { return "SOCKS5 127.0.0.1:9050"; }`
	tmp := t.TempDir() + "/invalid.pac"
	if err := os.WriteFile(tmp, []byte(pac), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := EvaluatorConfig{
		PACPath:        tmp,
		LocalIP:        "10.0.0.2",
		DNSTimeout:     100 * time.Millisecond,
		EvalTimeout:    100 * time.Millisecond,
		EnableDNSCache: false,
		FallbackRoute:  Route{Strategy: "DIRECT"},
	}
	_, err := NewEvaluator(cfg)
	if err == nil {
		t.Fatal("Expected error for invalid __PAC_RULES__")
	}
}

func TestEvaluatorCreatesDefaultPAC(t *testing.T) {
	dir := t.TempDir()
	pacPath := filepath.Join(dir, "nonexistent.pac")

	cfg := EvaluatorConfig{
		PACPath:        pacPath,
		LocalIP:        "10.0.0.2",
		DNSTimeout:     100 * time.Millisecond,
		EvalTimeout:    100 * time.Millisecond,
		EnableDNSCache: false,
		FallbackRoute:  Route{Strategy: "DIRECT"},
	}
	ev, err := NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator should succeed by creating default PAC: %v", err)
	}
	ev.Close()

	content, err := os.ReadFile(pacPath)
	if err != nil {
		t.Fatalf("Default PAC file should exist: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "FindProxyForURL") {
		t.Errorf("Default PAC should contain FindProxyForURL, got: %s", s)
	}
	if !strings.Contains(s, "__PAC_RULES__") {
		t.Errorf("Default PAC should contain __PAC_RULES__, got: %s", s)
	}
	if !strings.Contains(s, `{"version":1,"rules":[]}`) {
		t.Errorf("Default PAC should have empty rules, got: %s", s)
	}
}

func TestEvaluatorDoesNotOverwriteExistingPAC(t *testing.T) {
	dir := t.TempDir()
	pacPath := filepath.Join(dir, "existing.pac")

	// Create a valid native PAC with some rules
	rules := []gfw.Rule{
		{ID: "usr-1", Source: "user", Enabled: true, Match: "exact", Value: "example.com", Route: "SOCKS5 127.0.0.1:9050"},
	}
	pac := gfw.GeneratePAC(rules, nil, gfw.FWMeta{})
	if err := os.WriteFile(pacPath, []byte(pac), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := EvaluatorConfig{
		PACPath:        pacPath,
		LocalIP:        "10.0.0.2",
		DNSTimeout:     100 * time.Millisecond,
		EvalTimeout:    100 * time.Millisecond,
		EnableDNSCache: false,
		FallbackRoute:  Route{Strategy: "DIRECT"},
	}
	ev, err := NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	ev.Close()

	// File should be unchanged — not overwritten with default PAC
	content, err := os.ReadFile(pacPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	if strings.Contains(s, defaultPACContent) {
		t.Errorf("Existing PAC was overwritten with default PAC")
	}
	if !strings.Contains(s, "SOCKS5 127.0.0.1:9050") {
		t.Errorf("Existing PAC content was changed: %s", s)
	}
}

func TestCustomPAC_RoutesGoogleToTUN(t *testing.T) {
	dir := t.TempDir()
	pacPath := filepath.Join(dir, "custom.pac")

	// Create a PAC with a rule for google.com to TUN
	rules := []gfw.Rule{
		{ID: "1", Source: "user", Enabled: true, Match: "domain_suffix", Value: ".google.com", Route: "SOCKS5 127.0.0.1:1080"},
	}
	pac := gfw.GeneratePAC(rules, nil, gfw.FWMeta{})
	if err := os.WriteFile(pacPath, []byte(pac), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := EvaluatorConfig{
		PACPath:        pacPath,
		LocalIP:        "10.0.0.2",
		DNSTimeout:     100 * time.Millisecond,
		EvalTimeout:    100 * time.Millisecond,
		EnableDNSCache: false,
		FallbackRoute:  Route{Strategy: "DIRECT"},
	}
	ev, err := NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer ev.Close()

	hosts := []string{"google.com", "www.google.com", "mail.google.com"}
	for _, host := range hosts {
		route := ev.Evaluate(host)
		if route.Strategy != "SOCKS5" {
			t.Errorf("Custom PAC should return SOCKS5 for %s, got %v", host, route)
		}
	}
}

func TestDefaultPAC_RoutesOtherToDirect(t *testing.T) {
	dir := t.TempDir()
	pacPath := filepath.Join(dir, "auto.pac")

	cfg := EvaluatorConfig{
		PACPath:        pacPath,
		LocalIP:        "10.0.0.2",
		DNSTimeout:     100 * time.Millisecond,
		EvalTimeout:    100 * time.Millisecond,
		EnableDNSCache: false,
		FallbackRoute:  Route{Strategy: "DIRECT"},
	}
	ev, err := NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer ev.Close()

	hosts := []string{"example.com", "192.168.1.1", "localhost"}
	for _, host := range hosts {
		route := ev.Evaluate(host)
		if route.Strategy != "DIRECT" {
			t.Errorf("Default PAC should return DIRECT for %s, got %v", host, route)
		}
	}
}

func TestEvaluatorFailsWhenDirectoryMissing(t *testing.T) {
	pacPath := filepath.Join(t.TempDir(), "nonexistent_dir", "pac.js")

	cfg := EvaluatorConfig{
		PACPath:        pacPath,
		LocalIP:        "10.0.0.2",
		DNSTimeout:     100 * time.Millisecond,
		EvalTimeout:    100 * time.Millisecond,
		EnableDNSCache: false,
		FallbackRoute:  Route{Strategy: "DIRECT"},
	}
	_, err := NewEvaluator(cfg)
	if err == nil {
		t.Fatal("Expected error when parent directory doesn't exist")
	}
	if !strings.Contains(err.Error(), "create default pac file") {
		t.Errorf("Error should mention creating default PAC, got: %v", err)
	}
}
