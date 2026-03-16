package gfw

import (
	"bufio"
	"fmt"
	"strings"
)

// ParseGFWList parses decoded gfwlist text into structured rules.
// It handles AdBlock Plus filter syntax:
//
//	||domain     → domain suffix match
//	|url         → URL glob match
//	domain/path  → host glob match
//	@@||domain   → domain whitelist (DIRECT)
//	@@/regex     → regex whitelist (DIRECT, regex skipped)
//	! comment    → skipped
//	[section]    → skipped
//	/regex/      → skipped (out of scope)
func ParseGFWList(input string) []Rule {
	var rules []Rule
	id := 0

	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if line == "" {
			continue
		}

		// Skip comments
		if strings.HasPrefix(line, "!") {
			continue
		}

		// Skip section headers
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			continue
		}

		// Skip regex rules
		if strings.HasPrefix(line, "/") && strings.HasSuffix(line, "/") && len(line) > 2 {
			continue
		}

		rule := parseLine(line)
		if rule == nil {
			continue
		}

		id++
		rule.ID = fmt.Sprintf("gfw-%d", id)
		rule.Source = "gfw"
		rule.Enabled = true
		rules = append(rules, *rule)
	}

	return rules
}

func parseLine(line string) *Rule {
	// Check for exception rules (@@)
	isException := strings.HasPrefix(line, "@@")
	if isException {
		line = strings.TrimPrefix(line, "@@")
	}

	// Check for ||domain pattern
	if strings.HasPrefix(line, "||") {
		domain := strings.TrimPrefix(line, "||")
		// Remove path if present
		if idx := strings.IndexAny(domain, "/^"); idx >= 0 {
			domain = domain[:idx]
		}
		domain = strings.TrimSpace(domain)
		if domain == "" {
			return nil
		}
		// Must look like a domain (contains a dot or is an IP)
		if !strings.Contains(domain, ".") && !isIP(domain) {
			return nil
		}
		value := "." + domain
		if strings.HasPrefix(domain, ".") {
			value = domain
		}
		return &Rule{
			Match:  MatchDomainSuffix,
			Value:  value,
			Action: action(isException),
			Route:  route(isException),
		}
	}

	// Check for |url pattern (URL glob)
	if strings.HasPrefix(line, "|") {
		url := strings.TrimPrefix(line, "|")
		url = strings.TrimSpace(url)
		if url == "" {
			return nil
		}
		return &Rule{
			Match:  MatchURLGlob,
			Value:  url,
			Action: action(isException),
			Route:  route(isException),
		}
	}

	// Check for regex rules (already handled above, but double-check)
	if strings.HasPrefix(line, "/") && strings.HasSuffix(line, "/") {
		return nil
	}

	// Host glob pattern (default)
	value := line
	// Remove leading/trailing wildcards for cleaner matching
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	return &Rule{
		Match:  MatchHostGlob,
		Value:  normalizeGlob(value),
		Action: action(isException),
		Route:  route(isException),
	}
}

func action(isException bool) string {
	if isException {
		return "DIRECT"
	}
	return "PROXY"
}

func route(isException bool) string {
	if isException {
		return "DIRECT"
	}
	return "SOCKS5 127.0.0.1:1080" // default proxy route
}

// normalizeGlob ensures the pattern starts and ends with * for broad matching.
func normalizeGlob(pattern string) string {
	if !strings.HasPrefix(pattern, "*") {
		pattern = "*" + pattern
	}
	if !strings.HasSuffix(pattern, "*") {
		pattern = pattern + "*"
	}
	return pattern
}

// isIP checks if a string looks like an IP address (basic check).
func isIP(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}
