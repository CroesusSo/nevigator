package pac

import (
	"regexp"
	"strings"
	"testing"
)

// shellPatternToRegex converts a shell-style glob pattern to a regular expression.
// This helper is used in tests; the production code uses an inline version (globToRegex).
func shellPatternToRegex(pattern string) string {
	// Escape special regex characters, then replace glob wildcards with regex equivalents.
	re := regexp.QuoteMeta(pattern)
	re = strings.ReplaceAll(re, `\*`, `.*`)
	re = strings.ReplaceAll(re, `\?`, `.`)
	return "^" + re + "$"
}

func TestShellPatternToRegex(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{"*.com", `^.*\.com$`},
		{"?example", `^.example$`},
		{"a*b", `^a.*b$`},
		{"a?b", `^a.b$`},
	}
	for _, tt := range tests {
		got := shellPatternToRegex(tt.pattern)
		if got != tt.want {
			t.Errorf("shellPatternToRegex(%q) = %q, want %q", tt.pattern, got, tt.want)
		}
	}
}

func TestParsePACResult(t *testing.T) {
	tests := []struct {
		input string
		want  Route
	}{
		{"DIRECT", Route{Strategy: "DIRECT"}},
		{"SOCKS5 1.2.3.4:1080", Route{Strategy: "SOCKS5", Address: "1.2.3.4:1080"}},
		{"SOCKS5", Route{Strategy: "DIRECT"}}, // malformed
		{"PROXY foo:8080", Route{Strategy: "DIRECT"}},
		{"", Route{Strategy: "DIRECT"}},
		{"   ", Route{Strategy: "DIRECT"}},
		{"UNKNOWN", Route{Strategy: "DIRECT"}},
		{"EXTERNAL tor", Route{Strategy: "EXTERNAL", ExternalID: "tor"}},
		{"DEFAULT", Route{Strategy: "DEFAULT"}},
		{"EXTERNAL", Route{Strategy: "DIRECT"}}, // missing ID falls back to DIRECT
	}
	for _, tt := range tests {
		got := parsePACResult(tt.input)
		if got.Strategy != tt.want.Strategy || got.Address != tt.want.Address || got.ExternalID != tt.want.ExternalID {
			t.Errorf("parsePACResult(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
