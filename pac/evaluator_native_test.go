package pac

import (
	"os"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/gfw"
)

// TestEvaluatorNativeFallback tests that when __PAC_RULES__ is an invalid array (not an object), the evaluator falls back to JS.
func TestEvaluatorNativeFallback(t *testing.T) {
	t.Skip("native-only now; no JS fallback")
	// Use an array (invalid schema) but JS is valid
	pac := `var __PAC_RULES__ = []; function FindProxyForURL(url, host) { return "DIRECT"; }`
	tmp := t.TempDir() + "/malformed.pac"
	if err := os.WriteFile(tmp, []byte(pac), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := EvaluatorConfig{
		PACPath:       tmp,
		LocalIP:       "127.0.0.1",
		DNSTimeout:    100 * time.Millisecond,
		EvalTimeout:   100 * time.Millisecond,
		FallbackRoute: Route{Strategy: "DIRECT"},
		CacheEnabled:  false,
	}
	e, err := NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator failed: %v", err)
	}
	defer e.Close()
	// Should fall back to JS engine, return DIRECT
	r := e.Evaluate("example.com")
	if r.Strategy != "DIRECT" {
		t.Errorf("expected DIRECT, got %v", r)
	}
}

// TestEvaluatorNativeEngine tests that a PAC with valid __PAC_RULES__ (object schema) uses the native engine.
func TestEvaluatorNativeEngine(t *testing.T) {
	// Hardcoded valid RulesFile JSON object
	pac := `var __PAC_RULES__ = {"version":1,"rules":[{"id":"1","source":"test","enabled":true,"match":"exact","value":"exact.com","action":"PROXY","route":"TUN"},{"id":"2","source":"test","enabled":true,"match":"domain_suffix","value":".suffix.com","action":"PROXY","route":"SOCKS5 1.2.3.4:5678"}]}; function FindProxyForURL(url, host) { return 'DIRECT'; }`
	tmp := t.TempDir() + "/native.pac"
	if err := os.WriteFile(tmp, []byte(pac), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := EvaluatorConfig{
		PACPath:       tmp,
		LocalIP:       "127.0.0.1",
		DNSTimeout:    100 * time.Millisecond,
		EvalTimeout:   100 * time.Millisecond,
		FallbackRoute: Route{Strategy: "DIRECT"},
		CacheEnabled:  false,
	}
	e, err := NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer e.Close()
	// Verify native engine behavior
	if r := e.Evaluate("exact.com"); r.Strategy != "TUN" {
		t.Errorf("exact.com -> %v, want TUN", r)
	}
	if r := e.Evaluate("foo.suffix.com"); r.Strategy != "SOCKS5" {
		t.Errorf("foo.suffix.com -> %v, want SOCKS5", r)
	}
	if r := e.Evaluate("unknown.com"); r.Strategy != "DIRECT" {
		t.Errorf("unknown.com -> %v, want DIRECT", r)
	}
}

// TestEvaluatorCacheDefaults tests that default cache parameters are applied when cache is enabled but size/ttl not set.
func TestEvaluatorCacheDefaults(t *testing.T) {
	// Use valid RulesFile object
	pac := `var __PAC_RULES__ = {"version":1,"rules":[{"id":"1","source":"test","enabled":true,"match":"exact","value":"example.com","action":"PROXY","route":"TUN"}]}; function FindProxyForURL(url, host) { return 'DIRECT'; }`
	tmp := t.TempDir() + "/default.pac"
	if err := os.WriteFile(tmp, []byte(pac), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := EvaluatorConfig{
		PACPath:       tmp,
		LocalIP:       "127.0.0.1",
		FallbackRoute: Route{Strategy: "DIRECT"},
		CacheEnabled:  true, // enable but not specify max_size or ttl
	}
	e, err := NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer e.Close()
	if e.cache == nil {
		t.Fatal("cache not created")
	}
	// Check defaults
	if e.cache.maxSize != 10000 {
		t.Errorf("expected default maxSize 10000, got %d", e.cache.maxSize)
	}
	if e.cache.ttl != 10*time.Minute {
		t.Errorf("expected default TTL 10m, got %v", e.cache.ttl)
	}
}
// TestEvaluate_ExceptionOverridesBlacklist tests that GFW exception (DIRECT) takes precedence over blacklist (PROXY) for the same domain pattern.
func TestEvaluate_ExceptionOverridesBlacklist(t *testing.T) {
	// Create PAC with both an exception and a blacklist for .example.com
	gfwRules := []gfw.Rule{
		{ID: "ex", Source: "gfw", Enabled: true, Action: "DIRECT", Match: gfw.MatchDomainSuffix, Value: ".example.com", Route: "DIRECT"},
		{ID: "bl", Source: "gfw", Enabled: true, Action: "PROXY", Match: gfw.MatchDomainSuffix, Value: ".example.com", Route: "SOCKS5 1.2.3.4:1080"},
	}
	pac := gfw.GeneratePAC(nil, gfwRules, gfw.FWMeta{})
	tmp := t.TempDir() + "/test.pac"
	if err := os.WriteFile(tmp, []byte(pac), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := EvaluatorConfig{
		PACPath:       tmp,
		LocalIP:       "127.0.0.1",
		DNSTimeout:    100 * time.Millisecond,
		EvalTimeout:   100 * time.Millisecond,
		FallbackRoute: Route{Strategy: "DIRECT"},
		CacheEnabled:  false,
	}
	ev, err := NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer ev.Close()

	route := ev.Evaluate("sub.example.com")
	if route.Strategy != "DIRECT" {
		t.Errorf("exception should override blacklist, got %v", route)
	}
}
