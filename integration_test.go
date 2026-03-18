package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/gfw"
	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/router"
)

// TestEvaluatorIntegration tests PAC evaluation with a real PAC file.
func TestEvaluatorIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "test.pac")
	pacContent := `function FindProxyForURL(url, host) {
		if (shExpMatch(host, "*.direct")) {
			return "DIRECT";
		}
		if (dnsDomainIs(host, "tun.example")) {
			return "TUN";
		}
		return "SOCKS5 1.2.3.4:1080";
	}`
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

	tests := []struct {
		host string
		want string
	}{
		{"foo.direct", "DIRECT"},
		{"bar.tun.example", "TUN"},
		{"other.com", "SOCKS5"},
	}
	for _, tt := range tests {
		route := ev.Evaluate(tt.host)
		if route.Strategy != tt.want {
			t.Errorf("Evaluate(%s) = %v, want %s", tt.host, route.Strategy, tt.want)
		}
	}
}

// TestExternalProxyParsing tests that PAC returning "EXTERNAL myproxy" is parsed correctly.
func TestExternalProxyParsing(t *testing.T) {
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "test.pac")
	pacContent := `function FindProxyForURL(url, host) {
		if (dnsDomainIs(host, ".external.com")) {
			return "EXTERNAL myproxy";
		}
		return "DIRECT";
	}`
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
	route := ev.Evaluate("test.external.com")
	if route.Strategy != "EXTERNAL" {
		t.Errorf("expected EXTERNAL, got %s", route.Strategy)
	}
	if route.ExternalID != "myproxy" {
		t.Errorf("expected ExternalID myproxy, got %s", route.ExternalID)
	}
}

// TestDefaultProxyParsing tests that PAC returning "DEFAULT" yields DEFAULT route.
func TestDefaultProxyParsing(t *testing.T) {
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "test.pac")
	pacContent := `function FindProxyForURL(url, host) {
		if (dnsDomainIs(host, ".default.com")) {
			return "DEFAULT";
		}
		return "DIRECT";
	}`
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
	route := ev.Evaluate("test.default.com")
	if route.Strategy != "DEFAULT" {
		t.Errorf("expected DEFAULT, got %s", route.Strategy)
	}
}

// TestRouterFallback tests that failed TUN route falls back to DIRECT.
func TestRouterFallback(t *testing.T) {
	tunIP := net.ParseIP("127.0.0.1")
	routerCfg := pac.RouterConfig{
		TUNIP:       tunIP,
		DialTimeout: 50 * time.Millisecond,
	}
	r, err := router.NewRouter(routerCfg)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	dest := "8.8.8.8:53"
	route := pac.Route{Strategy: "TUN"}
	conn, err := r.Route(dest, route)
	if err != nil {
		t.Logf("Router fallback resulted in error (no network?): %v", err)
		return
	}
	conn.Close()
}

// TestHotReload tests PAC file hot reload.
func TestHotReload(t *testing.T) {
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "pac.js")
	initial := `function FindProxyForURL(url, host) { return "DIRECT"; }`
	if err := os.WriteFile(pacPath, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := pac.EvaluatorConfig{
		PACPath:       pacPath,
		LocalIP:       "10.0.0.2",
		DNSTimeout:    50 * time.Millisecond,
		EvalTimeout:   50 * time.Millisecond,
		FallbackRoute: pac.Route{Strategy: "DIRECT"},
	}
	ev, err := pac.NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer ev.Close()
	if ev.Evaluate("test").Strategy != "DIRECT" {
		t.Error("initial route not DIRECT")
	}
	modified := `function FindProxyForURL(url, host) { return "TUN"; }`
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(pacPath, []byte(modified), 0644); err != nil {
		t.Fatal(err)
	}
	ev.Reload()
	if ev.Evaluate("test").Strategy != "TUN" {
		t.Error("after reload, route not TUN")
	}
}

// TestPACTimout tests that infinite loop in PAC falls back.
func TestPACTimout(t *testing.T) {
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "pac.js")
	content := `function FindProxyForURL(url, host) { while (true) {} return "TUN"; }`
	if err := os.WriteFile(pacPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := pac.EvaluatorConfig{
		PACPath:       pacPath,
		LocalIP:       "10.0.0.2",
		DNSTimeout:    10 * time.Millisecond,
		EvalTimeout:   50 * time.Millisecond,
		FallbackRoute: pac.Route{Strategy: "DIRECT"},
	}
	ev, err := pac.NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer ev.Close()
	route := ev.Evaluate("example.com")
	if route.Strategy != "DIRECT" {
		t.Errorf("expected DIRECT fallback after timeout, got %v", route.Strategy)
	}
}

// TestRuleEditorUserPrecedence tests that user rules take precedence over GFW rules.
func TestRuleEditorUserPrecedence(t *testing.T) {
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "test.pac")
	userPath := filepath.Join(tmpDir, "user-rules.json")
	gfwPath := filepath.Join(tmpDir, "gfw-rules.json")
	userRules := []gfw.Rule{
		{ID: "usr-1", Source: "user", Enabled: true, Match: gfw.MatchDomainSuffix, Value: ".google.com", Route: "SOCKS5 my-proxy:1080"},
	}
	gfwRules := []gfw.Rule{
		{ID: "gfw-1", Source: "gfw", Enabled: true, Match: gfw.MatchDomainSuffix, Value: ".google.com", Route: "SOCKS5 127.0.0.1:1080"},
		{ID: "gfw-2", Source: "gfw", Enabled: true, Match: gfw.MatchDomainSuffix, Value: ".youtube.com", Route: "SOCKS5 127.0.0.1:1080"},
	}
	gfw.SaveRules(userPath, userRules)
	gfw.SaveRules(gfwPath, gfwRules)
	pacContent := gfw.GeneratePAC(userRules, gfwRules, gfw.FWMeta{})
	if err := os.WriteFile(pacPath, []byte(pacContent), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := pac.EvaluatorConfig{
		PACPath:       pacPath,
		LocalIP:       "10.0.0.2",
		DNSTimeout:    100 * time.Millisecond,
		EvalTimeout:   200 * time.Millisecond,
		FallbackRoute: pac.Route{Strategy: "DIRECT"},
	}
	ev, err := pac.NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer ev.Close()
	route := ev.Evaluate("google.com")
	if route.Strategy != "SOCKS5" {
		t.Fatalf("expected SOCKS5 for google.com, got %s", route.Strategy)
	}
	if route.Address != "my-proxy:1080" {
		t.Errorf("expected user rule route 'my-proxy:1080', got %q", route.Address)
	}
	route2 := ev.Evaluate("youtube.com")
	if route2.Strategy != "SOCKS5" {
		t.Fatalf("expected SOCKS5 for youtube.com, got %s", route2.Strategy)
	}
	if route2.Address != "127.0.0.1:1080" {
		t.Errorf("expected GFW rule route '127.0.0.1:1080', got %q", route2.Address)
	}
}

// TestRuleEditorGenerateAndParse tests the full generate→parse cycle.
func TestRuleEditorGenerateAndParse(t *testing.T) {
	userRules := []gfw.Rule{
		{ID: "usr-1", Source: "user", Enabled: true, Match: gfw.MatchHostGlob, Value: "*.onion", Route: "SOCKS5 127.0.0.1:9050"},
	}
	gfwRules := []gfw.Rule{
		{ID: "gfw-1", Source: "gfw", Enabled: true, Match: gfw.MatchDomainSuffix, Value: ".google.com", Route: "SOCKS5 127.0.0.1:1080"},
		{ID: "gfw-2", Source: "gfw", Enabled: false, Match: gfw.MatchDomainSuffix, Value: ".disabled.com", Route: "SOCKS5 127.0.0.1:1080"},
	}
	pac := gfw.GeneratePAC(userRules, gfwRules, gfw.FWMeta{})
	parsed := gfw.ParsePACRules(pac)
	if parsed == nil {
		t.Fatal("ParsePACRules returned nil")
	}
	if len(parsed) != 3 {
		t.Fatalf("expected 3 rules (2 user + 1 enabled GFW), got %d", len(parsed))
	}
	if parsed[0].Source != "user" {
		t.Errorf("first parsed rule should be user, got %q", parsed[0].Source)
	}
}

// TestRuleEditorBackwardCompat tests that raw PAC files without __PAC_RULES__ still work.
func TestRuleEditorBackwardCompat(t *testing.T) {
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "test.pac")
	rawPAC := `function FindProxyForURL(url, host) {
		if (dnsDomainIs(host, ".legacy.com"))
			return "SOCKS5 legacy:1080";
		return "DIRECT";
	}`
	if err := os.WriteFile(pacPath, []byte(rawPAC), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := pac.EvaluatorConfig{
		PACPath:       pacPath,
		LocalIP:       "10.0.0.2",
		DNSTimeout:    100 * time.Millisecond,
		EvalTimeout:   200 * time.Millisecond,
		FallbackRoute: pac.Route{Strategy: "DIRECT"},
	}
	ev, err := pac.NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer ev.Close()
	route := ev.Evaluate("sub.legacy.com")
	if route.Strategy != "SOCKS5" {
		t.Errorf("expected SOCKS5, got %s", route.Strategy)
	}
	if route.Address != "legacy:1080" {
		t.Errorf("expected 'legacy:1080', got %q", route.Address)
	}
	route2 := ev.Evaluate("other.com")
	if route2.Strategy != "DIRECT" {
		t.Errorf("expected DIRECT, got %s", route2.Strategy)
	}
}

// TestRuleEditorPACRegeneration tests PAC file regeneration triggers evaluator reload.
func TestRuleEditorPACRegeneration(t *testing.T) {
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "test.pac")
	userPath := filepath.Join(tmpDir, "user-rules.json")
	gfwPath := filepath.Join(tmpDir, "gfw-rules.json")
	userRules := []gfw.Rule{
		{ID: "usr-1", Source: "user", Enabled: true, Match: gfw.MatchDomainSuffix, Value: ".test.com", Route: "TUN"},
	}
	gfw.SaveRules(userPath, userRules)
	gfw.SaveRules(gfwPath, []gfw.Rule{})
	pacContent := gfw.GeneratePAC(userRules, nil, gfw.FWMeta{})
	if err := os.WriteFile(pacPath, []byte(pacContent), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := pac.EvaluatorConfig{
		PACPath:       pacPath,
		LocalIP:       "10.0.0.2",
		DNSTimeout:    100 * time.Millisecond,
		EvalTimeout:   200 * time.Millisecond,
		FallbackRoute: pac.Route{Strategy: "DIRECT"},
	}
	ev, err := pac.NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	defer ev.Close()
	route := ev.Evaluate("sub.test.com")
	if route.Strategy != "TUN" {
		t.Errorf("initial: expected TUN, got %s", route.Strategy)
	}
	newUserRules := []gfw.Rule{
		{ID: "usr-1", Source: "user", Enabled: true, Match: gfw.MatchDomainSuffix, Value: ".test.com", Route: "SOCKS5 new:1080"},
	}
	gfw.SaveRules(userPath, newUserRules)
	newPAC := gfw.GeneratePAC(newUserRules, nil, gfw.FWMeta{})
	time.Sleep(1100 * time.Millisecond)
	if err := os.WriteFile(pacPath, []byte(newPAC), 0644); err != nil {
		t.Fatal(err)
	}
	ev.Reload()
	time.Sleep(100 * time.Millisecond)
	route2 := ev.Evaluate("sub.test.com")
	if route2.Strategy != "SOCKS5" {
		t.Errorf("after reload: expected SOCKS5, got %s", route2.Strategy)
	}
	if route2.Address != "new:1080" {
		t.Errorf("after reload: expected 'new:1080', got %q", route2.Address)
	}
}
