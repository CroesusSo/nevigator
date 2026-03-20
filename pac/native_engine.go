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

func invert_segments(pattern string) []string {
	pattern = strings.ToLower(pattern)
	segments := strings.Split(pattern, ".")
	// reverse segments: mail.abc.com -> ["com","abc","mail"]
	for i, j := 0, len(segments)-1; i < j; i, j = i+1, j-1 {
		segments[i], segments[j] = segments[j], segments[i]
	}
	return segments
}

func (t *domainSuffixTrie) insert(pattern string, meta ruleMeta) {
	segments := invert_segments(pattern)

	node := t.root
	for i, seg := range segments {
		if seg == "*" {
			// handle catch all (*)
			if node.wildcards_child == nil {
				node.wildcards_child = newTrieNode()
			}
			node = node.wildcards_child
		} else if strings.ContainsAny(seg, "*?") {
			// handle partial globs
			if i == len(segments)-1 {
				// if it's last segment
				node.regex_patterns = append(node.regex_patterns, RegexRule{
					pattern: globToRegex(seg),
					rule:    &ruleMeta{index: meta.index, route: meta.route},
				})
				return
			}
			// TODO: we did not take care of complex globs ("a.*.c")
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

	segments := invert_segments(host)
	var last_rule *ruleMeta
	node := t.root

	for _, seg := range segments {
		// 1. Check for a rule at current level (LPM)
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
	case "TUN":
		return Route{Strategy: "TUN"}
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
		meta := ruleMeta{index: i, route: routeFromString(r.Route)}
		switch r.Match {
		case gfw.MatchExact:
			pattern := r.Value
			engine.suffixTrie.insert(pattern, meta)

		case gfw.MatchHostGlob:
			pattern := r.Value
			engine.suffixTrie.insert(pattern, meta)

		case gfw.MatchDomainSuffix:
			pattern := strings.TrimPrefix(r.Value, ".")
			engine.suffixTrie.insert(pattern, meta)

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
