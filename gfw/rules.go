package gfw

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// MatchType describes how a rule matches traffic.
type MatchType string

const (
	// MatchDomainSuffix matches hosts ending with the value (e.g., ".google.com").
	MatchDomainSuffix MatchType = "domain_suffix"
	// MatchHostGlob matches hosts using shell-style wildcards.
	MatchHostGlob MatchType = "host_glob"
	// MatchURLGlob matches full URLs using shell-style wildcards.
	MatchURLGlob MatchType = "url_glob"
	// MatchExact matches an exact host string.
	MatchExact MatchType = "exact"
)

// Rule represents a single routing rule.
type Rule struct {
	ID      string    `json:"id"`
	Source  string    `json:"source"` // "user" or "gfw"
	Enabled bool      `json:"enabled"`
	Match   MatchType `json:"match"`
	Value   string    `json:"value"`
	Action  string    `json:"action"` // "PROXY" or "DIRECT" (for exceptions)
	Route   string    `json:"route"`  // resolved route: "TUN", "SOCKS5 x:port"
}

// RulesFile represents the full rules document stored as JSON.
type RulesFile struct {
	Version int    `json:"version"`
	Rules   []Rule `json:"rules"`
}

// RuleStore manages reading and writing rule files.
type RuleStore struct {
	mu       sync.RWMutex
	filePath string
}

// NewRuleStore creates a new RuleStore for the given file path.
func NewRuleStore(filePath string) *RuleStore {
	return &RuleStore{filePath: filePath}
}

// Load reads rules from the JSON file. Returns empty slice if file doesn't exist.
func (s *RuleStore) Load() ([]Rule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Rule{}, nil
		}
		return nil, fmt.Errorf("read rules file: %v", err)
	}
	if len(data) == 0 {
		return []Rule{}, nil
	}

	var rf RulesFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("parse rules file: %v", err)
	}
	return rf.Rules, nil
}

// Save writes rules to the JSON file atomically (tmp + rename).
func (s *RuleStore) Save(rules []Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rf := RulesFile{
		Version: 1,
		Rules:   rules,
	}
	data, err := json.MarshalIndent(rf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal rules: %v", err)
	}

	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write tmp rules file: %v", err)
	}
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename rules file: %v", err)
	}
	return nil
}

// LoadRules is a convenience function to load rules from a file.
func LoadRules(path string) ([]Rule, error) {
	store := NewRuleStore(path)
	return store.Load()
}

// SaveRules is a convenience function to save rules to a file.
func SaveRules(path string, rules []Rule) error {
	store := NewRuleStore(path)
	return store.Save(rules)
}
