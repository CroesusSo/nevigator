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
	root *trieNode
}

type trieNode struct {
	children map[string]*trieNode
	rule     *ruleMeta
}

func newDomainSuffixTrie() *domainSuffixTrie {
	return &domainSuffixTrie{root: &trieNode{children: make(map[string]*trieNode)}}
}

func (t *domainSuffixTrie) insert(pattern string, meta ruleMeta) {
	labels := strings.Split(pattern, ".")
	for i, j := 0, len(labels)-1; i < j; i, j = i+1, j-1 {
		labels[i], labels[j] = labels[j], labels[i]
	}
	node := t.root
	for _, label := range labels {
		child, ok := node.children[label]
		if !ok {
			child = &trieNode{children: make(map[string]*trieNode)}
			node.children[label] = child
		}
		node = child
	}
	if node.rule == nil || meta.index < node.rule.index {
		node.rule = &ruleMeta{index: meta.index, route: meta.route}
	}
}

func (t *domainSuffixTrie) match(host string) *ruleMeta {
	host = strings.ToLower(host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	labels := strings.Split(host, ".")
	for i, j := 0, len(labels)-1; i < j; i, j = i+1, j-1 {
		labels[i], labels[j] = labels[j], labels[i]
	}
	bestIndex := int(^uint(0) >> 1)
	var bestRule *ruleMeta
	node := t.root
	for _, label := range labels {
		child, ok := node.children[label]
		if !ok {
			break
		}
		node = child
		if node.rule != nil && node.rule.index < bestIndex {
			bestIndex = node.rule.index
			bestRule = node.rule
		}
	}
	return bestRule
}

type ruleMetaRegex struct {
	meta  ruleMeta
	regex *regexp.Regexp
}

// NativeRuleEngine evaluates hosts using native data structures.
type NativeRuleEngine struct {
	exactMap   map[string]*ruleMeta
	suffixTrie *domainSuffixTrie
	hostGlobs  []ruleMetaRegex
	fallback   Route
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

// NewNativeRuleEngine builds a native rule engine from PAC content.
func NewNativeRuleEngine(pacContent string, fallback Route) (*NativeRuleEngine, error) {
	rules, err := parsePACRulesJSON(pacContent)
	if err != nil {
		return nil, err
	}
	engine := &NativeRuleEngine{
		exactMap:   make(map[string]*ruleMeta),
		suffixTrie: newDomainSuffixTrie(),
		hostGlobs:  make([]ruleMetaRegex, 0),
		fallback:   fallback,
	}
	globSeen := make(map[string]bool)
	for i, r := range rules {
		if !r.Enabled {
			continue
		}
		meta := ruleMeta{index: i, route: routeFromString(r.Route)}
		switch r.Match {
		case gfw.MatchExact:
			host := strings.ToLower(r.Value)
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			if existing, ok := engine.exactMap[host]; !ok || meta.index < existing.index {
				engine.exactMap[host] = &ruleMeta{index: meta.index, route: meta.route}
			}
		case gfw.MatchDomainSuffix:
			pattern := strings.TrimPrefix(r.Value, ".")
			pattern = strings.ToLower(pattern)
			engine.suffixTrie.insert(pattern, meta)
		case gfw.MatchHostGlob:
			pattern := r.Value
			if globSeen[pattern] {
				continue
			}
			regexStr := shellPatternToRegex(pattern)
			regex, err := regexp.Compile(regexStr)
			if err != nil {
				log.Printf("[WARN] Invalid host glob pattern %q: %v", pattern, err)
				continue
			}
			engine.hostGlobs = append(engine.hostGlobs, ruleMetaRegex{
				meta:  meta,
				regex: regex,
			})
			globSeen[pattern] = true
		case gfw.MatchURLGlob:
			log.Printf("[WARN] Ignoring MatchURLGlob rule for %s: URL-glob not supported in native engine", r.Value)
			continue
		}
	}
	return engine, nil
}

// Evaluate returns the best matching route for the given host.
func (e *NativeRuleEngine) Evaluate(host string) Route {
	host = strings.ToLower(host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	bestIndex := int(^uint(0) >> 1)
	var bestRoute Route
	if m, ok := e.exactMap[host]; ok && m.index < bestIndex {
		bestIndex = m.index
		bestRoute = m.route
	}
	if m := e.suffixTrie.match(host); m != nil && m.index < bestIndex {
		bestIndex = m.index
		bestRoute = m.route
	}
	for _, hg := range e.hostGlobs {
		if hg.regex.MatchString(host) {
			if hg.meta.index < bestIndex {
				bestIndex = hg.meta.index
				bestRoute = hg.meta.route
			}
			// continue to find lowest index among all matches (no break)
		}
	}
	if bestIndex == int(^uint(0)>>1) {
		return e.fallback
	}
	return bestRoute
}
