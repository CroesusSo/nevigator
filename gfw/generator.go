package gfw

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FWMeta contains metadata about the GFW list.
type FWMeta struct {
	Source               string `json:"source"`
	LastUpdated          string `json:"lastUpdated"`
	RuleCount            int    `json:"ruleCount"`
	DefaultExternalProxy string `json:"default_external_proxy,omitempty"`
	EnableURLGlob        bool   `json:"enable_url_glob,omitempty"`
}

// GeneratePAC merges user rules and GFW rules into PAC JavaScript.
// User rules are evaluated first (priority 1-99), GFW rules second (priority 100+).
func GeneratePAC(userRules []Rule, gfwRules []Rule, meta FWMeta) string {
	// Resolve routes: replace empty Route with DEFAULT if default proxy configured, else DIRECT
	resolve := func(r Rule) Rule {
		if r.Route == "" {
			if meta.DefaultExternalProxy != "" {
				r.Route = "DEFAULT"
			} else {
				r.Route = "DIRECT"
			}
		}
		return r
	}
	resolvedUser := make([]Rule, len(userRules))
	for i := range userRules {
		resolvedUser[i] = resolve(userRules[i])
	}
	resolvedGFW := make([]Rule, len(gfwRules))
	for i := range gfwRules {
		resolvedGFW[i] = resolve(gfwRules[i])
	}

	// Separate GFW rules: exceptions (Action == "DIRECT") and blacklist (Action != "DIRECT")
	var exceptions, blacklist []Rule
	for _, r := range resolvedGFW {
		if r.Action == "DIRECT" {
			exceptions = append(exceptions, r)
		} else {
			blacklist = append(blacklist, r)
		}
	}

	// Optionally filter URL-glob rules based on meta.EnableURLGlob
	if !meta.EnableURLGlob {
		filterURLGlob := func(rules []Rule) []Rule {
			filtered := make([]Rule, 0, len(rules))
			for _, r := range rules {
				if r.Match != MatchURLGlob {
					filtered = append(filtered, r)
				}
			}
			return filtered
		}
		resolvedUser = filterURLGlob(resolvedUser)
		exceptions = filterURLGlob(exceptions)
		blacklist = filterURLGlob(blacklist)
	}

	// Build combined rules in order: user rules, exceptions, blacklist
	allRules := make([]Rule, 0, len(resolvedUser)+len(exceptions)+len(blacklist))
	allRules = append(allRules, resolvedUser...)
	allRules = append(allRules, exceptions...)
	allRules = append(allRules, blacklist...)

	rf := RulesFile{Version: 1, Rules: allRules}
	jsonData, _ := json.Marshal(rf)

	// Write __PAC_RULES__ as a JS variable (not comment to avoid */ in data)
	sb := &strings.Builder{}
	sb.WriteString("var __PAC_RULES__ = ")
	sb.Write(jsonData)
	sb.WriteString(";\n\n")

	// Write generated JS
	sb.WriteString("// Generated from __PAC_RULES__ — do not edit manually\n")
	sb.WriteString("function FindProxyForURL(url, host) {\n")

	// Write conditions for each rule in order (they already have correct indices from allRules)
	for _, r := range allRules {
		if !r.Enabled {
			continue
		}
		cond := ruleToCondition(r)
		if cond == "" {
			continue
		}
		route := r.Route
		if route == "" {
			if meta.DefaultExternalProxy != "" {
				route = "DEFAULT"
			} else {
				route = "DIRECT"
			}
		}
		fmt.Fprintf(sb, "  if (%s)\n", cond)
		fmt.Fprintf(sb, "    return %q;\n", route)
	}

	// Fallback
	sb.WriteString("  return \"DIRECT\";\n")
	sb.WriteString("}\n")

	return sb.String()
}

// ruleToCondition converts a rule to a PAC JS condition expression.
func ruleToCondition(r Rule) string {
	switch r.Match {
	case MatchDomainSuffix:
		domain := strings.TrimPrefix(r.Value, ".")
		return fmt.Sprintf("dnsDomainIs(host, \".%s\") || host === %q", domain, domain)
	case MatchHostGlob:
		return fmt.Sprintf("shExpMatch(host, %q)", r.Value)
	case MatchURLGlob:
		return fmt.Sprintf("shExpMatch(url, %q)", r.Value)
	case MatchExact:
		return fmt.Sprintf("host === %q", r.Value)
	default:
		return ""
	}
}

// ParsePACRules extracts structured rules from a PAC file's __PAC_RULES__ variable.
// Supports both legacy /* __PAC_RULES__ */ and new var __PAC_RULES__ = formats.
func ParsePACRules(pacContent string) []Rule {
	// Try new format: var __PAC_RULES__ = {...};
	startMarker := "var __PAC_RULES__ = "
	start := strings.Index(pacContent, startMarker)
	if start >= 0 {
		start += len(startMarker)
		end := strings.Index(pacContent[start:], ";")
		if end < 0 {
			return nil
		}
		end += start
		jsonStr := pacContent[start:end]
		var rf RulesFile
		if err := json.Unmarshal([]byte(jsonStr), &rf); err != nil {
			return nil
		}
		return rf.Rules
	}

	// Fallback to legacy comment format
	const marker = "/* __PAC_RULES__\n"
	const endMarker = "\n*/"
	start = strings.Index(pacContent, marker)
	if start < 0 {
		return nil
	}
	start += len(marker)
	end := strings.Index(pacContent[start:], endMarker)
	if end < 0 {
		return nil
	}
	end += start
	jsonStr := pacContent[start:end]
	var rf RulesFile
	if err := json.Unmarshal([]byte(jsonStr), &rf); err != nil {
		return nil
	}
	return rf.Rules
}
