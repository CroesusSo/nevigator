package gfw

import (
	"crypto/rand"
	"fmt"
	"sync"
)

// RuleManager provides CRUD operations for user rules.
type RuleManager struct {
	mu    sync.Mutex
	store *RuleStore
}

// NewRuleManager creates a new RuleManager for the given rules file.
func NewRuleManager(filePath string) *RuleManager {
	return &RuleManager{
		store: NewRuleStore(filePath),
	}
}

// List returns all user rules.
func (m *RuleManager) List() ([]Rule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store.Load()
}

// Create adds a new user rule and saves.
func (m *RuleManager) Create(rule Rule) (Rule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rules, err := m.store.Load()
	if err != nil {
		return Rule{}, err
	}

	rule.ID = generateID("usr", rules)
	rule.Source = "user"
	rule.Enabled = true

	rules = append(rules, rule)
	if err := m.store.Save(rules); err != nil {
		return Rule{}, err
	}
	return rule, nil
}

// Update modifies an existing user rule.
func (m *RuleManager) Update(id string, updated Rule) (Rule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rules, err := m.store.Load()
	if err != nil {
		return Rule{}, err
	}

	idx := findRuleIndex(rules, id)
	if idx < 0 {
		return Rule{}, fmt.Errorf("rule not found: %s", id)
	}

	updated.ID = id
	updated.Source = "user"
	rules[idx] = updated

	if err := m.store.Save(rules); err != nil {
		return Rule{}, err
	}
	return updated, nil
}

// Delete removes a user rule by ID.
func (m *RuleManager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rules, err := m.store.Load()
	if err != nil {
		return err
	}

	idx := findRuleIndex(rules, id)
	if idx < 0 {
		return fmt.Errorf("rule not found: %s", id)
	}

	rules = append(rules[:idx], rules[idx+1:]...)
	return m.store.Save(rules)
}

// Toggle enables or disables a rule by ID.
func (m *RuleManager) Toggle(id string) (Rule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rules, err := m.store.Load()
	if err != nil {
		return Rule{}, err
	}

	idx := findRuleIndex(rules, id)
	if idx < 0 {
		return Rule{}, fmt.Errorf("rule not found: %s", id)
	}

	rules[idx].Enabled = !rules[idx].Enabled
	if err := m.store.Save(rules); err != nil {
		return Rule{}, err
	}
	return rules[idx], nil
}

// Reorder changes the order of user rules.
func (m *RuleManager) Reorder(ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rules, err := m.store.Load()
	if err != nil {
		return err
	}

	// Build lookup map
	ruleMap := make(map[string]Rule, len(rules))
	for _, r := range rules {
		ruleMap[r.ID] = r
	}

	// Rebuild in requested order
	var reordered []Rule
	seen := make(map[string]bool)
	for _, id := range ids {
		if r, ok := ruleMap[id]; ok {
			reordered = append(reordered, r)
			seen[id] = true
		}
	}

	// Append any rules not in the reorder list (preserve them)
	for _, r := range rules {
		if !seen[r.ID] {
			reordered = append(reordered, r)
		}
	}

	return m.store.Save(reordered)
}

func findRuleIndex(rules []Rule, id string) int {
	for i, r := range rules {
		if r.ID == id {
			return i
		}
	}
	return -1
}

// generateID creates a unique ID for a new rule.
func generateID(prefix string, existing []Rule) string {
	for {
		b := make([]byte, 4)
		rand.Read(b)
		candidate := fmt.Sprintf("%s-%x", prefix, b)
		if findRuleIndex(existing, candidate) < 0 {
			return candidate
		}
	}
}
