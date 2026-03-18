package gfw

import (
	"testing"
)

func TestParseDomainSuffix(t *testing.T) {
	tests := []struct {
		input string
		value string
		route string
	}{
		{"||google.com", ".google.com", ""},
		{"||youtube.com", ".youtube.com", ""},
		{"||.example.org", ".example.org", ""},
	}

	for _, tt := range tests {
		rules := ParseGFWList(tt.input)
		if len(rules) != 1 {
			t.Errorf("ParseGFWList(%q): expected 1 rule, got %d", tt.input, len(rules))
			continue
		}
		r := rules[0]
		if r.Match != MatchDomainSuffix {
			t.Errorf("ParseGFWList(%q): expected MatchDomainSuffix, got %s", tt.input, r.Match)
		}
		if r.Value != tt.value {
			t.Errorf("ParseGFWList(%q): expected value %q, got %q", tt.input, tt.value, r.Value)
		}
		if r.Route != tt.route {
			t.Errorf("ParseGFWList(%q): expected route %q, got %q", tt.input, tt.route, r.Route)
		}
		if r.Source != "gfw" {
			t.Errorf("ParseGFWList(%q): expected source 'gfw', got %q", tt.input, r.Source)
		}
	}
}

func TestParseDomainWithPort(t *testing.T) {
	rules := ParseGFWList("||example.com:8080")
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Match != MatchDomainSuffix {
		t.Errorf("expected MatchDomainSuffix, got %s", rules[0].Match)
	}
}

func TestParseException(t *testing.T) {
	tests := []struct {
		input string
		value string
	}{
		{"@@||internal.com", ".internal.com"},
		{"@@||local.net", ".local.net"},
	}

	for _, tt := range tests {
		rules := ParseGFWList(tt.input)
		if len(rules) != 1 {
			t.Errorf("ParseGFWList(%q): expected 1 rule, got %d", tt.input, len(rules))
			continue
		}
		r := rules[0]
		if r.Action != "DIRECT" {
			t.Errorf("ParseGFWList(%q): expected action DIRECT, got %q", tt.input, r.Action)
		}
		if r.Route != "DIRECT" {
			t.Errorf("ParseGFWList(%q): expected route DIRECT, got %q", tt.input, r.Route)
		}
	}
}

func TestParseURLGlob(t *testing.T) {
	rules := ParseGFWList("|http://example.com/path")
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Match != MatchURLGlob {
		t.Errorf("expected MatchURLGlob, got %s", rules[0].Match)
	}
	if rules[0].Value != "http://example.com/path" {
		t.Errorf("expected value 'http://example.com/path', got %q", rules[0].Value)
	}
}

func TestParseHostGlob(t *testing.T) {
	rules := ParseGFWList("google.com/search")
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Match != MatchHostGlob {
		t.Errorf("expected MatchHostGlob, got %s", rules[0].Match)
	}
}

func TestParseSkipped(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"comment", "! Title: GFWList"},
		{"section", "[AutoProxy 0.2.9]"},
		{"regex", "/^https?:\\/\\/[^/]+example/"},
		{"empty", ""},
		{"whitespace", "   "},
		{"comment with content", "! This is a comment"},
		{"section bracket", "[Adblock Plus 2.0]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := ParseGFWList(tt.input)
			if len(rules) != 0 {
				t.Errorf("ParseGFWList(%q): expected 0 rules, got %d", tt.input, len(rules))
			}
		})
	}
}

func TestParseMultipleLines(t *testing.T) {
	input := `! Title: GFWList
[AutoProxy 0.2.9]
||google.com
||youtube.com
@@||internal.corp.com
|http://blocked.net
/test-regex/
! Another comment
example.org
`

	rules := ParseGFWList(input)

	// Expected: google, youtube, internal (exception), blocked.net (url glob), example.org (host glob)
	// Skipped: comments, section, regex
	if len(rules) != 5 {
		t.Fatalf("expected 5 rules, got %d", len(rules))
	}

	// Verify rule ordering preserved
	if rules[0].Value != ".google.com" {
		t.Errorf("rule 0: expected .google.com, got %q", rules[0].Value)
	}
	if rules[1].Value != ".youtube.com" {
		t.Errorf("rule 1: expected .youtube.com, got %q", rules[1].Value)
	}
	if rules[2].Action != "DIRECT" {
		t.Errorf("rule 2: expected DIRECT (exception), got %q", rules[2].Action)
	}
	if rules[3].Match != MatchURLGlob {
		t.Errorf("rule 3: expected URL glob, got %s", rules[3].Match)
	}
	if rules[4].Match != MatchHostGlob {
		t.Errorf("rule 4: expected host glob, got %s", rules[4].Match)
	}
}

func TestParseIPRule(t *testing.T) {
	rules := ParseGFWList("||1.2.3.4")
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Match != MatchDomainSuffix {
		t.Errorf("expected MatchDomainSuffix for IP, got %s", rules[0].Match)
	}
	if rules[0].Value != ".1.2.3.4" {
		t.Errorf("expected value '.1.2.3.4', got %q", rules[0].Value)
	}
}

func TestParseRuleIDs(t *testing.T) {
	input := "||a.com\n||b.com\n||c.com"
	rules := ParseGFWList(input)

	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}

	expectedIDs := []string{"gfw-1", "gfw-2", "gfw-3"}
	for i, r := range rules {
		if r.ID != expectedIDs[i] {
			t.Errorf("rule %d: expected ID %q, got %q", i, expectedIDs[i], r.ID)
		}
	}
}

func TestParseEmpty(t *testing.T) {
	rules := ParseGFWList("")
	if len(rules) != 0 {
		t.Errorf("expected 0 rules from empty input, got %d", len(rules))
	}
}

func TestParseAllComments(t *testing.T) {
	input := "! comment 1\n! comment 2\n! comment 3"
	rules := ParseGFWList(input)
	if len(rules) != 0 {
		t.Errorf("expected 0 rules from all-comments input, got %d", len(rules))
	}
}

func TestParseWhitespaceTrimming(t *testing.T) {
	input := "  ||google.com  \n\n  ||youtube.com  \n"
	rules := ParseGFWList(input)
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].Value != ".google.com" {
		t.Errorf("rule 0: expected .google.com, got %q", rules[0].Value)
	}
	if rules[1].Value != ".youtube.com" {
		t.Errorf("rule 1: expected .youtube.com, got %q", rules[1].Value)
	}
}

func TestNormalizeGlob(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"google.com", "*google.com*"},
		{"*google.com", "*google.com*"},
		{"google.com*", "*google.com*"},
		{"*google.com*", "*google.com*"},
	}

	for _, tt := range tests {
		result := normalizeGlob(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeGlob(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}
