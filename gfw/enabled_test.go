package gfw

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetcherIsDisabled(t *testing.T) {
	f := NewFetcher("", "", 0)
	if f.IsDisabled() {
		t.Fatal("expected not disabled initially")
	}
	f.SetDisabled(true)
	if !f.IsDisabled() {
		t.Fatal("expected disabled after SetDisabled(true)")
	}
	f.SetDisabled(false)
	if f.IsDisabled() {
		t.Fatal("expected not disabled after SetDisabled(false)")
	}
}

func TestFetcherFetchWhenDisabled(t *testing.T) {
	f := NewFetcher("http://example.com/fake", "", 0)
	f.SetDisabled(true)
	ctx := context.Background()
	result := f.Fetch(ctx)
	if result.Error == nil {
		t.Error("expected error when disabled")
	}
}

func TestRegeneratePAC_DisabledExcludesGFW(t *testing.T) {
	dir := t.TempDir()
	userPath := filepath.Join(dir, "user.json")
	gfwPath := filepath.Join(dir, "gfw.json")
	pacPath := filepath.Join(dir, "proxy.pac")

	userRules := `{"version":1,"rules":[{"id":"u1","source":"user","enabled":true,"match":"exact","value":"user.com","action":"PROXY","route":"DIRECT"}]}`
	if err := os.WriteFile(userPath, []byte(userRules), 0644); err != nil {
		t.Fatal(err)
	}
	gfwRules := `{"version":1,"rules":[{"id":"g1","source":"gfw","enabled":true,"match":"domain_suffix","value":".example.com","action":"PROXY","route":"SOCKS5 1.2.3.4:1080"}]}`
	if err := os.WriteFile(gfwPath, []byte(gfwRules), 0644); err != nil {
		t.Fatal(err)
	}

	// enabled = false
	if err := RegeneratePAC(userPath, gfwPath, pacPath, "", false, false); err != nil {
		t.Fatalf("RegeneratePAC error: %v", err)
	}

	content, err := os.ReadFile(pacPath)
	if err != nil {
		t.Fatalf("read pac: %v", err)
	}
	pacStr := string(content)
	if !strings.Contains(pacStr, `"user.com"`) {
		t.Error("PAC missing user rule")
	}
	if strings.Contains(pacStr, `".example.com"`) {
		t.Error("PAC contains GFW rule when disabled")
	}
}
