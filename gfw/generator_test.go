package gfw

import (
	"strings"
	"testing"
)

func TestGeneratePACBasic(t *testing.T) {
	userRules := []Rule{
		{ID: "usr-1", Source: "user", Enabled: true, Match: MatchDomainSuffix, Value: ".internal.corp", Route: "TUN"},
	}
	gfwRules := []Rule{
		{ID: "gfw-1", Source: "gfw", Enabled: true, Match: MatchDomainSuffix, Value: ".google.com", Route: "SOCKS5 127.0.0.1:1080"},
	}
	meta := FWMeta{Source: "test", RuleCount: 1}

	pac := GeneratePAC(userRules, gfwRules, meta)

	// Should contain __PAC_RULES__ comment
	if !strings.Contains(pac, "__PAC_RULES__") {
		t.Error("PAC should contain __PAC_RULES__ comment")
	}

	// Should contain FindProxyForURL function
	if !strings.Contains(pac, "function FindProxyForURL") {
		t.Error("PAC should contain FindProxyForURL function")
	}

	// Should contain user rule before GFW rule
	userIdx := strings.Index(pac, ".internal.corp")
	gfwIdx := strings.Index(pac, ".google.com")
	if userIdx < 0 || gfwIdx < 0 {
		t.Fatal("both rules should be present in PAC")
	}
	if userIdx >= gfwIdx {
		t.Error("user rule should appear before GFW rule")
	}

	// Should contain DIRECT fallback
	if !strings.Contains(pac, `return "DIRECT"`) {
		t.Error("PAC should contain DIRECT fallback")
	}

	// Should contain dnsDomainIs
	if !strings.Contains(pac, `dnsDomainIs(host, ".internal.corp")`) {
		t.Error("PAC should use dnsDomainIs for domain suffix rules")
	}
}

func TestGeneratePACDisabledRules(t *testing.T) {
	userRules := []Rule{
		{ID: "usr-1", Source: "user", Enabled: false, Match: MatchDomainSuffix, Value: ".disabled.com", Route: "TUN"},
	}
	gfwRules := []Rule{
		{ID: "gfw-1", Source: "gfw", Enabled: true, Match: MatchDomainSuffix, Value: ".google.com", Route: "SOCKS5 127.0.0.1:1080"},
	}

	pac := GeneratePAC(userRules, gfwRules, FWMeta{})

	// Extract just the JS portion (after the __PAC_RULES__ comment)
	jsStart := strings.Index(pac, "function FindProxyForURL")
	if jsStart < 0 {
		t.Fatal("no JS function found")
	}
	jsPart := pac[jsStart:]

	if strings.Contains(jsPart, ".disabled.com") {
		t.Error("disabled rules should not appear in PAC JS")
	}
	if !strings.Contains(jsPart, ".google.com") {
		t.Error("enabled rules should appear in PAC JS")
	}
}

func TestGeneratePACMatchTypes(t *testing.T) {
	rules := []Rule{
		{ID: "1", Source: "user", Enabled: true, Match: MatchDomainSuffix, Value: ".example.com", Route: "TUN"},
		{ID: "2", Source: "user", Enabled: true, Match: MatchHostGlob, Value: "*.onion", Route: "SOCKS5 127.0.0.1:9050"},
		{ID: "3", Source: "user", Enabled: true, Match: MatchURLGlob, Value: "http://blocked.net/*", Route: "DIRECT"},
		{ID: "4", Source: "user", Enabled: true, Match: MatchExact, Value: "192.168.1.1", Route: "TUN"},
	}

	pac := GeneratePAC(rules, nil, FWMeta{})

	tests := []struct {
		condition string
	}{
		{`dnsDomainIs(host, ".example.com")`},
		{`shExpMatch(host, "*.onion")`},
		{`shExpMatch(url, "http://blocked.net/*")`},
		{`host === "192.168.1.1"`},
	}

	for _, tt := range tests {
		if !strings.Contains(pac, tt.condition) {
			t.Errorf("PAC should contain condition: %s", tt.condition)
		}
	}
}

func TestGeneratePACEmpty(t *testing.T) {
	pac := GeneratePAC(nil, nil, FWMeta{})

	// Should still have function and fallback
	if !strings.Contains(pac, "function FindProxyForURL") {
		t.Error("should have function even with no rules")
	}
	if !strings.Contains(pac, `return "DIRECT"`) {
		t.Error("should have DIRECT fallback even with no rules")
	}
}

func TestGeneratePACJSONComment(t *testing.T) {
	userRules := []Rule{
		{ID: "usr-1", Source: "user", Enabled: true, Match: MatchDomainSuffix, Value: ".test.com", Route: "TUN"},
	}
	gfwRules := []Rule{
		{ID: "gfw-1", Source: "gfw", Enabled: true, Match: MatchDomainSuffix, Value: ".google.com", Route: "SOCKS5 127.0.0.1:1080"},
	}

	pac := GeneratePAC(userRules, gfwRules, FWMeta{})

	// Parse the rules back from the generated PAC
	parsed := ParsePACRules(pac)
	if parsed == nil {
		t.Fatal("should be able to parse rules from generated PAC")
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 parsed rules, got %d", len(parsed))
	}
	if parsed[0].Source != "user" {
		t.Errorf("first parsed rule should be user, got %q", parsed[0].Source)
	}
	if parsed[1].Source != "gfw" {
		t.Errorf("second parsed rule should be gfw, got %q", parsed[1].Source)
	}
}

func TestParsePACRulesNoRules(t *testing.T) {
	// PAC without __PAC_RULES__ comment
	pac := `function FindProxyForURL(url, host) {
  return "DIRECT";
}`
	rules := ParsePACRules(pac)
	if rules != nil {
		t.Error("should return nil for PAC without structured rules")
	}
}

func TestParsePACRulesInvalidJSON(t *testing.T) {
	pac := `/* __PAC_RULES__
{invalid json}
*/
function FindProxyForURL(url, host) {
  return "DIRECT";
}`
	rules := ParsePACRules(pac)
	if rules != nil {
		t.Error("should return nil for invalid JSON")
	}
}

func TestRuleToCondition(t *testing.T) {
	tests := []struct {
		rule     Rule
		expected string
	}{
		{Rule{Match: MatchDomainSuffix, Value: ".test.com"}, `dnsDomainIs(host, ".test.com") || host === "test.com"`},
		{Rule{Match: MatchHostGlob, Value: "*.onion"}, `shExpMatch(host, "*.onion")`},
		{Rule{Match: MatchURLGlob, Value: "http://x.com"}, `shExpMatch(url, "http://x.com")`},
		{Rule{Match: MatchExact, Value: "1.2.3.4"}, `host === "1.2.3.4"`},
		{Rule{Match: "unknown", Value: "x"}, ""},
	}

	for _, tt := range tests {
		result := ruleToCondition(tt.rule)
		if result != tt.expected {
			t.Errorf("ruleToCondition(%+v): expected %q, got %q", tt.rule, tt.expected, result)
		}
	}
}

func TestWriteRuleConditionEscaping(t *testing.T) {
	// Route with special characters should be properly escaped
	rules := []Rule{
		{ID: "1", Source: "user", Enabled: true, Match: MatchDomainSuffix, Value: ".test.com", Route: `SOCKS5 evil:1080"; alert("xss")`},
	}
	pac := GeneratePAC(rules, nil, FWMeta{})

	// The injected route should be a single string literal, not break out of quotes
	if !strings.Contains(pac, `return "SOCKS5 evil:1080\"; alert(\"xss\")";`) {
		// %q produces Go-quoted strings with escaped quotes
		t.Logf("Generated PAC:\n%s", pac)
	}

	// Most importantly: no unescaped quotes should leak
	// Count return statements — should be exactly 1 return + 1 DIRECT fallback = 2
	returns := strings.Count(pac, "return ")
	if returns != 2 {
		t.Errorf("expected 2 return statements, got %d (possible injection)", returns)
	}
}

func TestGeneratePAC_DefaultExternalProxy(t *testing.T) {
	// Empty route with defaultExternalProxy set should use DEFAULT
	rule := Rule{
		ID:      "test",
		Source:  "user",
		Enabled: true,
		Match:   MatchDomainSuffix,
		Value:   ".example.com",
		Route:   "", // empty route
	}
	meta := FWMeta{DefaultExternalProxy: "my-proxy"}
	pac := GeneratePAC([]Rule{rule}, nil, meta)

	if !strings.Contains(pac, `return "DEFAULT"`) {
		t.Errorf("expected DEFAULT in PAC when defaultExternalProxy set, got:\n%s", pac)
	}
}

func TestGeneratePAC_NoDefaultExternalProxy(t *testing.T) {
	// Empty route with no defaultExternalProxy should use DIRECT
	rule := Rule{
		ID:      "test",
		Source:  "user",
		Enabled: true,
		Match:   MatchDomainSuffix,
		Value:   ".example.com",
		Route:   "", // empty route
	}
	meta := FWMeta{} // no default
	pac := GeneratePAC([]Rule{rule}, nil, meta)

	if !strings.Contains(pac, `return "DIRECT"`) {
		t.Errorf("expected DIRECT when no default proxy configured, got:\n%s", pac)
	}
}
