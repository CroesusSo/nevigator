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
	// Create PAC with rules:
	// - host glob *.direct -> DIRECT
	// - domain suffix .proxy.example -> SOCKS5 proxy
	userRules := []gfw.Rule{
		{ID: "r1", Source: "test", Enabled: true, Match: gfw.MatchHostGlob, Value: "*.direct", Route: "DIRECT"},
		{ID: "r2", Source: "test", Enabled: true, Match: gfw.MatchDomainSuffix, Value: ".proxy.example", Route: "SOCKS5 1.2.3.4:1080"},
	}
	pacContent := gfw.GeneratePAC(userRules, nil, gfw.FWMeta{})
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
		{"bar.proxy.example", "SOCKS5"},
		{"other.com", "DIRECT"}, // fallback
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
	rule := gfw.Rule{
		ID:      "ext",
		Source:  "user",
		Enabled: true,
		Match:   gfw.MatchDomainSuffix,
		Value:   ".external.com",
		Route:   "EXTERNAL myproxy",
	}
	pacContent := gfw.GeneratePAC([]gfw.Rule{rule}, nil, gfw.FWMeta{})
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
	rule := gfw.Rule{
		ID:      "def",
		Source:  "user",
		Enabled: true,
		Match:   gfw.MatchDomainSuffix,
		Value:   ".default.com",
		Route:   "DEFAULT",
	}
	pacContent := gfw.GeneratePAC([]gfw.Rule{rule}, nil, gfw.FWMeta{})
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

// TestRouterFallsBackToDirect verifies that a failed SOCKS5 route retries via
// DIRECT, as documented: "Route fallback: if chosen route fails (not DIRECT),
// automatically retries DIRECT."
//
// The upstream here is a closed loopback port, so the SOCKS5 dial fails
// immediately without touching the network, and the destination is a local
// listener so the DIRECT fallback can be observed succeeding.
func TestRouterFallsBackToDirect(t *testing.T) {
	// Destination that DIRECT can reach.
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("starting destination: %v", err)
	}
	defer target.Close()

	// A SOCKS5 upstream that is guaranteed to refuse: bind then close.
	dead, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving dead upstream port: %v", err)
	}
	deadAddr := dead.Addr().String()
	dead.Close()

	r, err := router.NewRouter(pac.RouterConfig{DialTimeout: 500 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	route := pac.Route{Strategy: "SOCKS5", Address: deadAddr}
	conn, err := r.Route(target.Addr().String(), route)
	if err != nil {
		t.Fatalf("expected DIRECT fallback to succeed, got error: %v", err)
	}
	defer conn.Close()

	// The returned conn must be the direct one, not a SOCKS5 tunnel.
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Errorf("fallback connection not usable: %v", err)
	}
}

// TestRouterFallbackFailsWhenBothRoutesFail checks that the fallback does not
// mask an unreachable destination: with both the SOCKS5 upstream and DIRECT
// unavailable, Route must return an error mentioning the original cause.
func TestRouterFallbackFailsWhenBothRoutesFail(t *testing.T) {
	dead, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving dead upstream port: %v", err)
	}
	deadAddr := dead.Addr().String()
	dead.Close()

	// A destination that nothing is listening on.
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving destination port: %v", err)
	}
	dest := closed.Addr().String()
	closed.Close()

	r, err := router.NewRouter(pac.RouterConfig{DialTimeout: 300 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	route := pac.Route{Strategy: "SOCKS5", Address: deadAddr}
	if _, err := r.Route(dest, route); err == nil {
		t.Fatal("expected an error when both the route and the DIRECT fallback fail")
	}
}

// TestRouterDirectRouteDoesNotRecurse checks that a failing DIRECT route
// returns its own error instead of retrying through the fallback.
//
// The destination is a loopback address in TEST-NET-1 (192.0.2.0/24), which is
// unroutable, so the dial blocks until DialTimeout rather than being refused
// immediately. If the fallback re-entered, the call would block for two full
// timeouts, which the assertion below detects.
func TestRouterDirectRouteDoesNotRecurse(t *testing.T) {
	const dialTimeout = 300 * time.Millisecond
	r, err := router.NewRouter(pac.RouterConfig{DialTimeout: dialTimeout})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	start := time.Now()
	_, err = r.Route("192.0.2.1:9", pac.Route{Strategy: "DIRECT"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error for an unreachable DIRECT destination")
	}
	// One timeout, not two. Allow slack for scheduler noise but well under 2x.
	if elapsed > 2*dialTimeout {
		t.Errorf("DIRECT failure took %v (> 2x dialTimeout %v), so the fallback re-entered",
			elapsed, dialTimeout)
	}
}

// TestHotReload tests PAC file hot reload.
func TestHotReload(t *testing.T) {
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "pac.js")
	// initial: no rules, returns DIRECT (fallback)
	initial := gfw.GeneratePAC(nil, nil, gfw.FWMeta{})
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
	// modified: rule that matches everything -> SOCKS5
	rule := gfw.Rule{
		ID:      "all",
		Source:  "user",
		Enabled: true,
		Match:   gfw.MatchHostGlob,
		Value:   "*",
		Route:   "SOCKS5 127.0.0.1:1080",
	}
	modified := gfw.GeneratePAC([]gfw.Rule{rule}, nil, gfw.FWMeta{})
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(pacPath, []byte(modified), 0644); err != nil {
		t.Fatal(err)
	}
	ev.Reload()
	time.Sleep(100 * time.Millisecond)
	if ev.Evaluate("test").Strategy != "SOCKS5" {
		t.Error("after reload, route not SOCKS5")
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

// TestRuleEditorPACRegeneration tests PAC file regeneration triggers evaluator reload.
func TestRuleEditorPACRegeneration(t *testing.T) {
	tmpDir := t.TempDir()
	pacPath := filepath.Join(tmpDir, "test.pac")
	userPath := filepath.Join(tmpDir, "user-rules.json")
	gfwPath := filepath.Join(tmpDir, "gfw-rules.json")
	userRules := []gfw.Rule{
		{ID: "usr-1", Source: "user", Enabled: true, Match: gfw.MatchDomainSuffix, Value: ".test.com", Route: "SOCKS5 old:1080"},
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
	if route.Strategy != "SOCKS5" {
		t.Errorf("initial: expected SOCKS5, got %s", route.Strategy)
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
