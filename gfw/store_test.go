package gfw

import (
	"path/filepath"
	"testing"
)

func TestRuleManagerCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")
	mgr := NewRuleManager(path)

	rule, err := mgr.Create(Rule{
		Match: MatchDomainSuffix,
		Value: ".test.com",
		Route: "SOCKS5 127.0.0.1:1080",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rule.ID == "" {
		t.Error("expected non-empty ID")
	}
	if rule.Source != "user" {
		t.Errorf("expected source 'user', got %q", rule.Source)
	}
	if !rule.Enabled {
		t.Error("expected enabled=true")
	}

	// Verify it persists
	rules, err := mgr.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
}

func TestRuleManagerUpdate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")
	mgr := NewRuleManager(path)

	created, err := mgr.Create(Rule{
		Match: MatchDomainSuffix,
		Value: ".test.com",
		Route: "SOCKS5 127.0.0.1:1080",
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := mgr.Update(created.ID, Rule{
		Match: MatchDomainSuffix,
		Value: ".updated.com",
		Route: "SOCKS5 10.0.0.1:1080",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Value != ".updated.com" {
		t.Errorf("expected .updated.com, got %q", updated.Value)
	}
	if updated.ID != created.ID {
		t.Error("ID should be preserved")
	}
}

func TestRuleManagerUpdateNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")
	mgr := NewRuleManager(path)

	_, err := mgr.Update("nonexistent", Rule{})
	if err == nil {
		t.Error("expected error for nonexistent rule")
	}
}

func TestRuleManagerDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")
	mgr := NewRuleManager(path)

	created, err := mgr.Create(Rule{Match: MatchExact, Value: "test", Route: "DIRECT"})
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.Delete(created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	rules, err := mgr.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules after delete, got %d", len(rules))
	}
}

func TestRuleManagerToggle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")
	mgr := NewRuleManager(path)

	created, err := mgr.Create(Rule{Match: MatchExact, Value: "test", Route: "DIRECT"})
	if err != nil {
		t.Fatal(err)
	}
	if !created.Enabled {
		t.Fatal("expected initially enabled")
	}

	toggled, err := mgr.Toggle(created.ID)
	if err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if toggled.Enabled {
		t.Error("expected disabled after toggle")
	}

	toggled2, err := mgr.Toggle(created.ID)
	if err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if !toggled2.Enabled {
		t.Error("expected enabled after second toggle")
	}
}

func TestRuleManagerReorder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")
	mgr := NewRuleManager(path)

	// Create 3 rules
	r1, _ := mgr.Create(Rule{Match: MatchExact, Value: "a", Route: "TUN"})
	r2, _ := mgr.Create(Rule{Match: MatchExact, Value: "b", Route: "TUN"})
	r3, _ := mgr.Create(Rule{Match: MatchExact, Value: "c", Route: "TUN"})

	// Reorder: c, a, b
	if err := mgr.Reorder([]string{r3.ID, r1.ID, r2.ID}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}

	rules, _ := mgr.List()
	if rules[0].Value != "c" || rules[1].Value != "a" || rules[2].Value != "b" {
		t.Errorf("unexpected order: %s, %s, %s", rules[0].Value, rules[1].Value, rules[2].Value)
	}
}

func TestRuleManagerReorderPartial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")
	mgr := NewRuleManager(path)

	// Only reorder r2 and r3; r1 should be appended
	_, _ = mgr.Create(Rule{Match: MatchExact, Value: "a", Route: "TUN"})
	r2, _ := mgr.Create(Rule{Match: MatchExact, Value: "b", Route: "TUN"})
	r3, _ := mgr.Create(Rule{Match: MatchExact, Value: "c", Route: "TUN"})

	// Only reorder r2 and r3; first rule should be appended
	if err := mgr.Reorder([]string{r3.ID, r2.ID}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}

	rules, _ := mgr.List()
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}
	// r3, r2, r1 (r1 appended)
	if rules[0].Value != "c" || rules[1].Value != "b" || rules[2].Value != "a" {
		t.Errorf("unexpected order: %s, %s, %s", rules[0].Value, rules[1].Value, rules[2].Value)
	}
}
