package pac

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvaluator(t *testing.T) {
	content := `function FindProxyForURL(url, host) { return "TUN"; }`
	tmp, err := os.CreateTemp(t.TempDir(), "pac*.js")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmp.Close()

	cfg := EvaluatorConfig{
		PACPath:        tmp.Name(),
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
	if route.Strategy != "TUN" {
		t.Errorf("Expected TUN, got %v", route)
	}
}

func TestEvaluatorSyntaxError(t *testing.T) {
	content := `function FindProxyForURL { return "TUN"; }` // missing parentheses
	tmp, err := os.CreateTemp(t.TempDir(), "pac*.js")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmp.Close()

	cfg := EvaluatorConfig{
		PACPath:        tmp.Name(),
		LocalIP:        "10.0.0.2",
		DNSTimeout:     100 * time.Millisecond,
		EvalTimeout:    100 * time.Millisecond,
		EnableDNSCache: false,
		FallbackRoute:  Route{Strategy: "DIRECT"},
	}
	ev, err := NewEvaluator(cfg)
	_ = err // syntax error at startup causes NewEvaluator to return error
	// If ev is nil, skip further checks
	if ev == nil {
		t.Skip("Evaluator failed to create due to syntax error, which is expected")
	}
	defer ev.Close()

	route := ev.Evaluate("example.com")
	if route.Strategy != "DIRECT" {
		t.Errorf("Expected fallback DIRECT, got %v", route)
	}
}

func TestEvaluatorTimeout(t *testing.T) {
	// PAC that loops forever
	content := `function FindProxyForURL(url, host) { while (true) {} return "TUN"; }`
	tmp, err := os.CreateTemp(t.TempDir(), "pac*.js")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmp.Close()

	cfg := EvaluatorConfig{
		PACPath:        tmp.Name(),
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
	if route.Strategy != "DIRECT" {
		t.Errorf("Expected fallback DIRECT after timeout, got %v", route)
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
	defer ev.Close()

	// File should now exist
	content, err := os.ReadFile(pacPath)
	if err != nil {
		t.Fatalf("Default PAC file should exist: %v", err)
	}
	if !strings.Contains(string(content), "FindProxyForURL") {
		t.Errorf("Default PAC should contain FindProxyForURL, got: %s", content)
	}
}

func TestEvaluatorDoesNotOverwriteExistingPAC(t *testing.T) {
	dir := t.TempDir()
	pacPath := filepath.Join(dir, "existing.pac")

	customPAC := `function FindProxyForURL(url, host) { return "SOCKS5 127.0.0.1:9050"; }`
	if err := os.WriteFile(pacPath, []byte(customPAC), 0644); err != nil {
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

	// File should be unchanged — not overwritten with default PAC
	content, err := os.ReadFile(pacPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), defaultPACContent) {
		t.Errorf("Existing PAC was overwritten with default PAC")
	}
	if !strings.Contains(string(content), "SOCKS5 127.0.0.1:9050") {
		t.Errorf("Existing PAC content was changed: %s", content)
	}
}

func TestDefaultPAC_RoutesGoogleToTUN(t *testing.T) {
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

	hosts := []string{"google.com", "www.google.com", "mail.google.com"}
	for _, host := range hosts {
		route := ev.Evaluate(host)
		if route.Strategy != "TUN" {
			t.Errorf("Default PAC should return TUN for %s, got %v", host, route)
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
