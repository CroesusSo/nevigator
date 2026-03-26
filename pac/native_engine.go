package pac

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"regexp"
	"strings"

	"github.com/yourorg/proxy-pac/gfw"
)

// ruleMeta stores metadata about a rule during engine construction.
type ruleMeta struct {
	index int
	route Route
}

// domainSuffixTrie is a reversed-label trie for domain suffix matching.
type domainSuffixTrie struct {
	root *TrieNode
}

type RegexRule struct {
	pattern *regexp.Regexp
	rule    *ruleMeta
}

type TrieNode struct {
	// exactMap
	children map[string]*TrieNode
	// child of (*)
	wildcards_child *TrieNode
	// compiled regex patterns
	regex_patterns []RegexRule

	// proxy result if rule ends exactly at the node
	rule *ruleMeta
}

func newTrieNode() *TrieNode {
	return &TrieNode{
		children: make(map[string]*TrieNode),
	}
}

func newDomainSuffixTrie() *domainSuffixTrie {
	return &domainSuffixTrie{root: newTrieNode()}
}

func globToRegex(glob string) *regexp.Regexp {
	// Escape special regex chars, then swap glob wildcards for regex equivalents
	re := regexp.QuoteMeta(glob)
	re = strings.ReplaceAll(re, "\\*", ".*")
	re = strings.ReplaceAll(re, "\\?", ".")
	return regexp.MustCompile("^" + re + "$")
}

func get_segments(pattern string, reverse bool) []string {
	pattern = strings.ToLower(pattern)
	segments := strings.Split(pattern, ".")
	if reverse {
		// reverse segments: mail.abc.com -> ["com","abc","mail"]
		for i, j := 0, len(segments)-1; i < j; i, j = i+1, j-1 {
			segments[i], segments[j] = segments[j], segments[i]
		}
	}
	return segments
}

// Simplified logic: Split only on dots that aren't inside regex parens/brackets
func splitDomainPattern(pattern string) []string {
	var segments []string
	var current []rune
	runes := []rune(pattern)

	brackets, parens, braces := 0, 0, 0
	escaped := false

	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if escaped {
			current = append(current, r)
			escaped = false
			continue
		}

		switch r {
		case '\\':
			escaped = true
			current = append(current, r)
		case '[':
			brackets++
			current = append(current, r)
		case ']':
			brackets--
			current = append(current, r)
		case '(':
			parens++
			current = append(current, r)
		case ')':
			parens--
			current = append(current, r)
		case '{':
			braces++
			current = append(current, r)
		case '}':
			braces--
			current = append(current, r)
		case '.':
			// Is this a structural dot or a regex dot?
			isSeparator := true

			// 1. If inside any brackets/parens, it's NOT a separator
			if brackets > 0 || parens > 0 || braces > 0 {
				isSeparator = false
			}

			// 2. Lookahead: If the NEXT char is a quantifier, the dot is REGEX
			if i+1 < len(runes) {
				next := runes[i+1]
				if next == '*' || next == '+' || next == '?' || next == '{' {
					isSeparator = false
				}
			}

			if isSeparator {
				segments = append(segments, string(current))
				current = []rune{}
			} else {
				current = append(current, r)
			}
		default:
			current = append(current, r)
		}
	}

	if len(current) > 0 {
		segments = append(segments, string(current))
	}
	return segments
}

func (t *domainSuffixTrie) insert(segments []string, meta ruleMeta) {
	node := t.root

	// proceed in reverse order www.google.com -> "com" "google" "www"
	for i := len(segments) - 1; i >= 0; i-- {
		seg := segments[i]
		if seg == "*" {
			// handle catch all (*)
			// if it's last segment
			if i == 0 {
				if node.wildcards_child == nil {
					node.wildcards_child = newTrieNode()
				}
				node = node.wildcards_child
			} else {
				log.Printf("[WARN] Ignoring middle * rule for pattern %q at segment %s", segments, seg)
				return
			}
		} else if strings.ContainsAny(seg, "*?") {
			// handle partial globs i.e ay-*.example.com
			if i == 0 {
				// if it's last segment (we proceed in reverse order)
				node.regex_patterns = append(node.regex_patterns, RegexRule{
					pattern: globToRegex(seg),
					rule:    &ruleMeta{index: meta.index, route: meta.route},
				})
				return
			} else {
				// TODO: we did not take care of complex globs ("a.ay-*.c")
				log.Printf("[WARN] Ignoring middle Glob rule for %p:(%s)", segments, seg)
				return
			}
		} else {
			// handle exact map
			if _, ok := node.children[seg]; !ok {
				node.children[seg] = newTrieNode()
			}
			node = node.children[seg]
		}
	}
	// set leaf rule for exact match
	if node.rule == nil {
		node.rule = &ruleMeta{index: meta.index, route: meta.route}
	}
}

func (t *domainSuffixTrie) match(host string, deepest bool) *ruleMeta {
	host = strings.ToLower(host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	segments := get_segments(host, false)
	var last_rule *ruleMeta
	node := t.root

	// proceed in reverse order www.google.com -> "com" "google" "www"
	for i := len(segments) - 1; i >= 0; i-- {
		seg := segments[i]
		// 1. Check for a rule at current depth (LPM)
		if node.rule != nil {
			last_rule = node.rule
		}
		// 2. Try exact match
		if next, ok := node.children[seg]; ok {
			node = next
			continue
		}
		// 3. Try Regex/Glob
		for _, regex_rule := range node.regex_patterns {
			if regex_rule.pattern.MatchString(seg) {
				return regex_rule.rule
			}
		}
		// 4. Try Catch all wildcard
		if node.wildcards_child != nil {
			node = node.wildcards_child
			continue
		}

		// no more match at this depth
		break
	}

	if node.rule != nil {
		return node.rule
	}
	return last_rule
}

func parsePACRulesJSON(pacContent string) ([]gfw.Rule, error) {
	startMarker := "var __PAC_RULES__ = "
	start := strings.Index(pacContent, startMarker)
	if start < 0 {
		return nil, fmt.Errorf("__PAC_RULES__ not found")
	}
	start += len(startMarker)
	end := strings.Index(pacContent[start:], ";")
	if end < 0 {
		return nil, fmt.Errorf("__PAC_RULES__ not terminated")
	}
	end += start
	jsonStr := pacContent[start:end]
	var rf gfw.RulesFile
	if err := json.Unmarshal([]byte(jsonStr), &rf); err != nil {
		return nil, fmt.Errorf("unmarshal __PAC_RULES__: %w", err)
	}
	return rf.Rules, nil
}

func routeFromString(s string) Route {
	s = strings.TrimSpace(s)
	if s == "" {
		return Route{Strategy: "DIRECT"}
	}
	tokens := strings.Fields(s)
	if len(tokens) == 0 {
		return Route{Strategy: "DIRECT"}
	}
	switch strings.ToUpper(tokens[0]) {
	case "SOCKS5":
		if len(tokens) >= 2 {
			return Route{Strategy: "SOCKS5", Address: tokens[1]}
		}
		return Route{Strategy: "DIRECT"}
	case "EXTERNAL":
		if len(tokens) >= 2 {
			return Route{Strategy: "EXTERNAL", ExternalID: tokens[1]}
		}
		return Route{Strategy: "DIRECT"}
	case "DEFAULT":
		return Route{Strategy: "DEFAULT"}
	case "DIRECT":
		return Route{Strategy: "DIRECT"}
	default:
		return Route{Strategy: "DIRECT"}
	}
}

// NativeRuleEngine evaluates hosts using native data structures.
// IMPORTANT: hostGlobs slice is built in ascending index order (by rule index).
// This invariant enables short-circuit evaluation in Evaluate().
type NativeRuleEngine struct {
	suffixTrie    *domainSuffixTrie
	fallback      Route
	enableDeepest bool
}

// NewNativeRuleEngine builds a native rule engine from PAC content.
func NewNativeRuleEngine(pacContent string, fallback Route, enableDeepest bool) (*NativeRuleEngine, error) {
	rules, err := parsePACRulesJSON(pacContent)
	if err != nil {
		return nil, err
	}

	engine := &NativeRuleEngine{
		suffixTrie:    newDomainSuffixTrie(),
		fallback:      fallback,
		enableDeepest: enableDeepest,
	}

	for i, r := range rules {
		if !r.Enabled {
			continue
		}
		// Reject TUN routes (removed feature)
		if strings.ToUpper(strings.TrimSpace(r.Route)) == "TUN" {
			return nil, fmt.Errorf("PAC contains unsupported TUN route at rule ID %s. TUN routing has been removed; use SOCKS5 or EXTERNAL proxy instead.", r.ID)
		}
		meta := ruleMeta{index: i, route: routeFromString(r.Route)}
		switch r.Match {
		case gfw.MatchExact:
			pattern := r.Value
			segments := get_segments(pattern, false)
			engine.suffixTrie.insert(segments, meta)

		case gfw.MatchDomainSuffix:
			pattern := strings.TrimPrefix(r.Value, ".")
			segments := get_segments(pattern, false)
			engine.suffixTrie.insert(segments, meta)

		case gfw.MatchHostGlob:
			pattern := r.Value
			segments := splitDomainPattern(pattern)
			engine.suffixTrie.insert(segments, meta)

		case gfw.MatchURLGlob:
			log.Printf("[WARN] Ignoring MatchURLGlob rule for %s: URL-glob not supported in native engine", r.Value)
			continue
		}
	}
	return engine, nil
}

// Evaluate returns the best matching route for the given host.
func (e *NativeRuleEngine) Evaluate(host string) Route {
	rule := e.suffixTrie.match(host, e.enableDeepest)
	if rule != nil {
		return rule.route
	}
	return e.fallback
}
