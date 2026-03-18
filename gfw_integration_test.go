package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/gfw"
	"github.com/yourorg/proxy-pac/pac"
)

// TestGFWBlacklistWithDefaultProxy tests that GFW blacklist rules generate PAC returning "DEFAULT" when default proxy is configured
func TestGFWBlacklistWithDefaultProxy(t *testing.T) {
	tmpDir := t.TempDir()
	userPath := filepath.Join(tmpDir, "user-rules.json")
	gfwPath := filepath.Join(tmpDir, "gfw-rules.json")
	pacPath := filepath.Join(tmpDir, "test.pac")

	// Create GFW blacklist rule (no @@ prefix)
	gfwRules := []gfw.Rule{
		{ID: "gfw-1", Source: "gfw", Enabled: true, Match: gfw.MatchDomainSuffix, Value: ".example.com", Action: "PROXY", Route: ""},
	}
	gfw.SaveRules(gfwPath, gfwRules)

	// No user rules
	userRules := []gfw.Rule{}
	gfw.SaveRules(userPath, userRules)

	// Generate PAC with default proxy configured
	pacContent := gfw.GeneratePAC(userRules, gfwRules, gfw.FWMeta{
		Source:               "test",
		DefaultExternalProxy: "my-proxy",
	})
	if err := os.WriteFile(pacPath, []byte(pacContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Test PAC evaluation
	cfg := pac.EvaluatorConfig{
		PACPath:        pacPath,
		LocalIP:        "10.0.0.2",
		DNSTimeout:     100 * time.Millisecond,
		EvalTimeout:    100 * time.Millisecond,
		EnableDNSCache: false,
		FallbackRoute:  pac.Route{Strategy: "DIRECT"},
	}
	ev, err := pac.NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer ev.Close()

	route := ev.Evaluate("test.example.com")
	if route.Strategy != "DEFAULT" {
		t.Errorf("expected DEFAULT strategy for GFW blacklist with default proxy, got %s", route.Strategy)
	}
}

// TestGFWBlacklistWithoutDefaultProxy tests that GFW blacklist rules generate PAC returning "DIRECT" when no default proxy is configured
func TestGFWBlacklistWithoutDefaultProxy(t *testing.T) {
	tmpDir := t.TempDir()
	userPath := filepath.Join(tmpDir, "user-rules.json")
	gfwPath := filepath.Join(tmpDir, "gfw-rules.json")
	pacPath := filepath.Join(tmpDir, "test.pac")

	// Create GFW blacklist rule (no @@ prefix)
	gfwRules := []gfw.Rule{
		{ID: "gfw-1", Source: "gfw", Enabled: true, Match: gfw.MatchDomainSuffix, Value: ".example.com", Action: "PROXY", Route: ""},
	}
	gfw.SaveRules(gfwPath, gfwRules)

	// No user rules
	userRules := []gfw.Rule{}
	gfw.SaveRules(userPath, userRules)

	// Generate PAC with NO default proxy configured
	pacContent := gfw.GeneratePAC(userRules, gfwRules, gfw.FWMeta{
		Source: "test",
		// DefaultExternalProxy is empty
	})
	if err := os.WriteFile(pacPath, []byte(pacContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Test PAC evaluation
	cfg := pac.EvaluatorConfig{
		PACPath:        pacPath,
		LocalIP:        "10.0.0.2",
		DNSTimeout:     100 * time.Millisecond,
		EvalTimeout:    100 * time.Millisecond,
		EnableDNSCache: false,
		FallbackRoute:  pac.Route{Strategy: "DIRECT"},
	}
	ev, err := pac.NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer ev.Close()

	route := ev.Evaluate("test.example.com")
	if route.Strategy != "DIRECT" {
		t.Errorf("expected DIRECT strategy for GFW blacklist without default proxy, got %s", route.Strategy)
	}
}

// TestGFWWhitelistUnchanged tests that GFW whitelist rules always generate PAC returning "DIRECT"
func TestGFWWhitelistUnchanged(t *testing.T) {
	tmpDir := t.TempDir()
	userPath := filepath.Join(tmpDir, "user-rules.json")
	gfwPath := filepath.Join(tmpDir, "gfw-rules.json")
	pacPath := filepath.Join(tmpDir, "test.pac")

	// Create GFW whitelist rule (@@ prefix) - let parser set the Route correctly
	gfwInput := "@@||internal.com"
	gfwRules := gfw.ParseGFWList(gfwInput)
	if len(gfwRules) != 1 {
		t.Fatalf("expected 1 rule from %q, got %d", gfwInput, len(gfwRules))
	}
	gfw.SaveRules(gfwPath, gfwRules)

	// No user rules
	userRules := []gfw.Rule{}
	gfw.SaveRules(userPath, userRules)

	// Test WITH default proxy configured
	pacContent := gfw.GeneratePAC(userRules, gfwRules, gfw.FWMeta{
		Source:               "test",
		DefaultExternalProxy: "my-proxy",
	})
	if err := os.WriteFile(pacPath, []byte(pacContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := pac.EvaluatorConfig{
		PACPath:        pacPath,
		LocalIP:        "10.0.0.2",
		DNSTimeout:     100 * time.Millisecond,
		EvalTimeout:    100 * time.Millisecond,
		EnableDNSCache: false,
		FallbackRoute:  pac.Route{Strategy: "DIRECT"},
	}
	ev, err := pac.NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer ev.Close()

	route := ev.Evaluate("test.internal.com")
	if route.Strategy != "DIRECT" {
		t.Errorf("expected DIRECT strategy for GFW whitelist (with default proxy), got %s", route.Strategy)
	}

	// Test WITHOUT default proxy configured
	pacContent = gfw.GeneratePAC(userRules, gfwRules, gfw.FWMeta{
		Source: "test",
		// DefaultExternalProxy is empty
	})
	if err := os.WriteFile(pacPath, []byte(pacContent), 0644); err != nil {
		t.Fatal(err)
	}

	ev, err = pac.NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer ev.Close()

	route = ev.Evaluate("test.internal.com")
	if route.Strategy != "DIRECT" {
		t.Errorf("expected DIRECT strategy for GFW whitelist (without default proxy), got %s", route.Strategy)
	}
}
