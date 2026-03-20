package pac

import (
	"encoding/json"
	"testing"

	"github.com/yourorg/proxy-pac/gfw"
)

// makeRule creates a gfw.Rule for testing.
func makeRule(value string, match gfw.MatchType, routeStr string, enabled bool) gfw.Rule {
	return gfw.Rule{
		ID:      "test",
		Source:  "test",
		Enabled: enabled,
		Match:   match,
		Value:   value,
		Action:  "PROXY",
		Route:   routeStr,
	}
}

// makeEngineWithRules creates a NativeRuleEngine from a slice of rules.
func makeEngineWithRules(rules []gfw.Rule) *NativeRuleEngine {
	rf := gfw.RulesFile{Version: 1, Rules: rules}
	jsonData, _ := json.Marshal(rf)
	pac := "var __PAC_RULES__ = " + string(jsonData) + ";\nfunction FindProxyForURL(url, host) {}"
	engine, _ := NewNativeRuleEngine(pac, Route{Strategy: "DIRECT"}, false)
	return engine
}

func TestRouteFromString(t *testing.T) {
	tests := []struct {
		input string
		want  Route
	}{
		{"TUN", Route{Strategy: "TUN"}},
		{"SOCKS5 127.0.0.1:1080", Route{Strategy: "SOCKS5", Address: "127.0.0.1:1080"}},
		{" EXTERNAL proxy1 ", Route{Strategy: "EXTERNAL", ExternalID: "proxy1"}},
		{"DEFAULT", Route{Strategy: "DEFAULT"}},
		{"DIRECT", Route{Strategy: "DIRECT"}},
		{"", Route{Strategy: "DIRECT"}},
		{"unknown", Route{Strategy: "DIRECT"}},
		{"SOCKS5", Route{Strategy: "DIRECT"}},
	}
	for _, tt := range tests {
		got := routeFromString(tt.input)
		if got != tt.want {
			t.Errorf("routeFromString(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestExactMap(t *testing.T) {
	rules := []gfw.Rule{
		makeRule("example.com", gfw.MatchExact, "TUN", true),
		makeRule("EXAMPLE.COM:8080", gfw.MatchExact, "SOCKS5 127.0.0.1:1080", true), // duplicate with port
		makeRule("test.com", gfw.MatchExact, "DIRECT", true),
	}
	engine := makeEngineWithRules(rules)

	// exact map should normalize host and strip port
	got1, ok := engine.exactMap["example.com"]
	if !ok || got1.route.Strategy != "TUN" {
		t.Error("expected example.com -> TUN")
	}
	// duplicate with higher index should not override
	_, ok = engine.exactMap["example.com"]
	if !ok {
		t.Error("example.com should still exist")
	}
	// test.com
	got2, ok := engine.exactMap["test.com"]
	if !ok || got2.route.Strategy != "DIRECT" {
		t.Error("expected test.com -> DIRECT")
	}
}

func TestSuffixTrie(t *testing.T) {
	rules := []gfw.Rule{
		makeRule(".google.com", gfw.MatchDomainSuffix, "TUN", true),
		makeRule(".co.uk", gfw.MatchDomainSuffix, "SOCKS5 1.2.3.4:5678", true),
		makeRule(".uk", gfw.MatchDomainSuffix, "DIRECT", true),
	}
	engine := makeEngineWithRules(rules)

	// google.com should match .google.com
	if r := engine.Evaluate("www.google.com"); r.Strategy != "TUN" {
		t.Errorf("www.google.com -> %v, want TUN", r)
	}
	// mail.google.com also matches .google.com
	if r := engine.Evaluate("mail.google.com"); r.Strategy != "TUN" {
		t.Errorf("mail.google.com -> %v, want TUN", r)
	}
	// example.co.uk should match .co.uk (more specific) over .uk
	if r := engine.Evaluate("example.co.uk"); r.Strategy != "SOCKS5" {
		t.Errorf("example.co.uk -> %v, want SOCKS5", r)
	}
	// test.uk should match .uk
	if r := engine.Evaluate("test.uk"); r.Strategy != "DIRECT" {
		t.Errorf("test.uk -> %v, want DIRECT", r)
	}
	// case insensitivity
	if r := engine.Evaluate("WWW.GOOGLE.COM"); r.Strategy != "TUN" {
		t.Errorf("WWW.GOOGLE.COM -> %v, want TUN", r)
	}
}

func TestSuffixTrieDedup(t *testing.T) {
	// If two rules have same suffix, earliest index wins
	rules := []gfw.Rule{
		makeRule(".example.com", gfw.MatchDomainSuffix, "TUN", true),    // index 0
		makeRule(".example.com", gfw.MatchDomainSuffix, "SOCKS5", true), // index 1, should be ignored
	}
	engine := makeEngineWithRules(rules)
	if r := engine.Evaluate("foo.example.com"); r.Strategy != "TUN" {
		t.Errorf("foo.example.com -> %v, want TUN", r)
	}
}

func TestHostGlobs(t *testing.T) {
	rules := []gfw.Rule{
		makeRule("*.google.com", gfw.MatchHostGlob, "TUN", true),
		makeRule("*.compute.amazonaws.com", gfw.MatchHostGlob, "SOCKS5 1.2.3.4:5678", true),
		makeRule("*.google.com", gfw.MatchHostGlob, "DIRECT", true), // duplicate, should be ignored
	}
	engine := makeEngineWithRules(rules)

	// Match www.google.com
	if r := engine.Evaluate("www.google.com"); r.Strategy != "TUN" {
		t.Errorf("www.google.com -> %v, want TUN", r)
	}
	// Match a compute.amazonaws.com host
	if r := engine.Evaluate("myinstance.compute.amazonaws.com"); r.Strategy != "SOCKS5" {
		t.Errorf("myinstance.compute.amazonaws.com -> %v, want SOCKS5", r)
	}
	// Duplicate pattern not added, so no issue
}

func TestHostGlobsOrdering(t *testing.T) {
	// Two different globs that both match the same host, earliest index wins
	rules := []gfw.Rule{
		makeRule("*.example.com", gfw.MatchHostGlob, "TUN", true), // index 0
		makeRule("www.*.com", gfw.MatchHostGlob, "SOCKS5", true),  // index 1, also matches www.example.com
	}
	engine := makeEngineWithRules(rules)
	// Both patterns match www.example.com; earliest index (TUN) should win
	if r := engine.Evaluate("www.example.com"); r.Strategy != "TUN" {
		t.Errorf("www.example.com -> %v, want TUN", r)
	}
	// Only second pattern matches sub.example.com
	if r := engine.Evaluate("sub.example.com"); r.Strategy != "TUN" {
		t.Errorf("sub.example.com -> %v, want TUN", r)
	}
}

func TestURLGlobsIgnored(t *testing.T) {
	rules := []gfw.Rule{
		makeRule("http://*", gfw.MatchURLGlob, "TUN", true),
	}
	engine := makeEngineWithRules(rules)
	// No rules should be added, engine should fallback
	if r := engine.Evaluate("example.com"); r.Strategy != "DIRECT" {
		t.Errorf("example.com -> %v, want DIRECT (fallback)", r)
	}
}

func TestMixedRules(t *testing.T) {
	rules := []gfw.Rule{
		makeRule("exact.com", gfw.MatchExact, "TUN", true),
		makeRule(".suffix.com", gfw.MatchDomainSuffix, "SOCKS5 1.2.3.4:5678", true),
		makeRule("*.glob.com", gfw.MatchHostGlob, "DEFAULT", true),
	}
	engine := makeEngineWithRules(rules)

	// Exact match wins for exact.com
	if r := engine.Evaluate("exact.com"); r.Strategy != "TUN" {
		t.Errorf("exact.com -> %v, want TUN", r)
	}
	// Suffix match for foo.suffix.com
	if r := engine.Evaluate("foo.suffix.com"); r.Strategy != "SOCKS5" {
		t.Errorf("foo.suffix.com -> %v, want SOCKS5", r)
	}
	// Glob match for www.glob.com
	if r := engine.Evaluate("www.glob.com"); r.Strategy != "DEFAULT" {
		t.Errorf("www.glob.com -> %v, want DEFAULT", r)
	}
	// No match falls back
	if r := engine.Evaluate("unknown.com"); r.Strategy != "DIRECT" {
		t.Errorf("unknown.com -> %v, want DIRECT", r)
	}
}

func TestPortStripping(t *testing.T) {
	// Exact rule without port, host with port should still match
	rules := []gfw.Rule{
		makeRule("example.com", gfw.MatchExact, "TUN", true),
	}
	engine := makeEngineWithRules(rules)
	if r := engine.Evaluate("example.com:8080"); r.Strategy != "TUN" {
		t.Errorf("example.com:8080 -> %v, want TUN", r)
	}
}

func TestExact_IPv6(t *testing.T) {
	// IPv6 exact matching with and without port
	rules := []gfw.Rule{
		makeRule("2001:db8::1", gfw.MatchExact, "TUN", true),
		makeRule("::1", gfw.MatchExact, "SOCKS5 127.0.0.1:1080", true),
	}
	engine := makeEngineWithRules(rules)

	// Plain IPv6 literal
	if r := engine.Evaluate("2001:db8::1"); r.Strategy != "TUN" {
		t.Errorf("2001:db8::1 -> %v, want TUN", r)
	}
	// IPv6 with port in brackets (caller should have stripped port)
	if r := engine.Evaluate("[2001:db8::1]:443"); r.Strategy != "TUN" {
		t.Errorf("[2001:db8::1]:443 -> %v, want TUN", r)
	}
	// Localhost IPv6
	if r := engine.Evaluate("::1"); r.Strategy != "SOCKS5" {
		t.Errorf("::1 -> %v, want SOCKS5", r)
	}
	// IPv6 with brackets and port
	if r := engine.Evaluate("[::1]:8080"); r.Strategy != "SOCKS5" {
		t.Errorf("[::1]:8080 -> %v, want SOCKS5", r)
	}
}

func TestHostGlobs_IPv6(t *testing.T) {
	// Host glob pattern matching for IPv6
	rules := []gfw.Rule{
		makeRule("2001:db8::*", gfw.MatchHostGlob, "TUN", true),
		makeRule("::*", gfw.MatchHostGlob, "DIRECT", true),
	}
	engine := makeEngineWithRules(rules)

	// Pattern 2001:db8::*
	if r := engine.Evaluate("2001:db8::1"); r.Strategy != "TUN" {
		t.Errorf("2001:db8::1 -> %v, want TUN", r)
	}
	if r := engine.Evaluate("2001:db8::abcd"); r.Strategy != "TUN" {
		t.Errorf("2001:db8::abcd -> %v, want TUN", r)
	}
	// Pattern ::* matches any IPv6 in :: subnet
	if r := engine.Evaluate("::1"); r.Strategy != "DIRECT" {
		t.Errorf("::1 -> %v, want DIRECT", r)
	}
	if r := engine.Evaluate("::ffff:192.0.2.1"); r.Strategy != "DIRECT" {
		t.Errorf("::ffff:192.0.2.1 -> %v, want DIRECT", r)
	}
	// Non-matching IPv6
	if r := engine.Evaluate("fe80::1"); r.Strategy != "DIRECT" {
		t.Errorf("fe80::1 -> %v, want DIRECT (fallback)", r)
	}
}

func TestSuffix_IPv6_NoMatch(t *testing.T) {
	// Domain suffix should not match IPv6 addresses
	rules := []gfw.Rule{
		makeRule(".example.com", gfw.MatchDomainSuffix, "TUN", true),
	}
	engine := makeEngineWithRules(rules)

	// IPv6 should not match domain suffix
	if r := engine.Evaluate("2001:db8::1"); r.Strategy != "DIRECT" {
		t.Errorf("2001:db8::1 -> %v, want DIRECT (no match)", r)
	}
	if r := engine.Evaluate("::1"); r.Strategy != "DIRECT" {
		t.Errorf("::1 -> %v, want DIRECT (no match)", r)
	}
}

func TestHostGlobs_Priority(t *testing.T) {
	// With removal of break, earliest index among all matching globs wins
	rules := []gfw.Rule{
		makeRule("*.example.com", gfw.MatchHostGlob, "TUN", true), // index 0
		makeRule("www.*.com", gfw.MatchHostGlob, "SOCKS5", true),  // index 1
		makeRule("*", gfw.MatchHostGlob, "DEFAULT", true),         // index 2, matches everything
	}
	engine := makeEngineWithRules(rules)

	// www.example.com matches all three; index 0 (TUN) should win
	if r := engine.Evaluate("www.example.com"); r.Strategy != "TUN" {
		t.Errorf("www.example.com -> %v, want TUN", r)
	}
	// sub.example.com matches first and third; index 0 wins
	if r := engine.Evaluate("sub.example.com"); r.Strategy != "TUN" {
		t.Errorf("sub.example.com -> %v, want TUN", r)
	}
	// example.org matches only the catch-all (*) → DEFAULT
	if r := engine.Evaluate("example.org"); r.Strategy != "DEFAULT" {
		t.Errorf("example.org -> %v, want DEFAULT", r)
	}
}
