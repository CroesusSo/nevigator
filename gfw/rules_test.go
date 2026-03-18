package gfw

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRuleJSONRoundTrip(t *testing.T) {
	rules := []Rule{
		{ID: "usr-001", Source: "user", Enabled: true, Match: MatchDomainSuffix, Value: ".internal.corp", Action: "PROXY", Route: "TUN"},
		{ID: "usr-002", Source: "user", Enabled: true, Match: MatchHostGlob, Value: "*.onion", Action: "PROXY", Route: "SOCKS5 127.0.0.1:9050"},
		{ID: "gfw-001", Source: "gfw", Enabled: true, Match: MatchDomainSuffix, Value: ".google.com", Action: "PROXY", Route: "SOCKS5 127.0.0.1:1080"},
		{ID: "gfw-002", Source: "gfw", Enabled: false, Match: MatchExact, Value: "1.2.3.4", Action: "PROXY", Route: "SOCKS5 127.0.0.1:1080"},
	}

	data, err := json.Marshal(RulesFile{Version: 1, Rules: rules})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var rf RulesFile
	if err := json.Unmarshal(data, &rf); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(rf.Rules) != len(rules) {
		t.Fatalf("expected %d rules, got %d", len(rules), len(rf.Rules))
	}

	for i, r := range rf.Rules {
		if r.ID != rules[i].ID {
			t.Errorf("rule %d: expected ID %q, got %q", i, rules[i].ID, r.ID)
		}
		if r.Source != rules[i].Source {
			t.Errorf("rule %d: expected Source %q, got %q", i, rules[i].Source, r.Source)
		}
		if r.Match != rules[i].Match {
			t.Errorf("rule %d: expected Match %q, got %q", i, rules[i].Match, r.Match)
		}
		if r.Value != rules[i].Value {
			t.Errorf("rule %d: expected Value %q, got %q", i, rules[i].Value, r.Value)
		}
		if r.Route != rules[i].Route {
			t.Errorf("rule %d: expected Route %q, got %q", i, rules[i].Route, r.Route)
		}
	}
}

func TestRuleStoreLoadSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")

	store := NewRuleStore(path)

	// Load from non-existent file returns empty
	rules, err := store.Load()
	if err != nil {
		t.Fatalf("load non-existent: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected 0 rules, got %d", len(rules))
	}

	// Save rules
	input := []Rule{
		{ID: "1", Source: "user", Enabled: true, Match: MatchDomainSuffix, Value: ".test.com", Action: "PROXY", Route: "TUN"},
	}
	if err := store.Save(input); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify file exists and is valid JSON
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var rf RulesFile
	if err := json.Unmarshal(data, &rf); err != nil {
		t.Fatalf("file is not valid JSON: %v", err)
	}

	// Load back
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(loaded))
	}
	if loaded[0].Value != ".test.com" {
		t.Errorf("expected value .test.com, got %q", loaded[0].Value)
	}
}

func TestRuleStoreAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")

	store := NewRuleStore(path)

	// Save should not leave tmp file behind
	if err := store.Save([]Rule{{ID: "1", Source: "user"}}); err != nil {
		t.Fatalf("save: %v", err)
	}

	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); err == nil {
		t.Error("tmp file should not exist after save")
	}
}

func TestConvenienceFunctions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")

	rules := []Rule{{ID: "1", Source: "user", Enabled: true, Match: MatchExact, Value: "test", Route: "DIRECT"}}
	if err := SaveRules(path, rules); err != nil {
		t.Fatalf("SaveRules: %v", err)
	}

	loaded, err := LoadRules(path)
	if err != nil {
		t.Fatalf("LoadRules: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Value != "test" {
		t.Errorf("unexpected loaded rules: %+v", loaded)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")

	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	rules, err := LoadRules(path)
	if err != nil {
		t.Fatalf("LoadRules: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected 0 rules from empty file, got %d", len(rules))
	}
}
