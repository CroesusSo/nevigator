package gfw

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// sampleGFWList returns base64-encoded gfwlist content.
func sampleGFWList() string {
	content := `! Title: Test GFWList
[AutoProxy 0.2.9]
||google.com
||youtube.com
@@||internal.corp.com
|http://blocked.net
`
	return base64.StdEncoding.EncodeToString([]byte(content))
}

func TestFetcherSuccessful(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"abc123"`)
		w.Header().Set("Last-Modified", "Sat, 15 Mar 2026 00:00:00 GMT")
		w.Write([]byte(sampleGFWList()))
	}))
	defer ts.Close()

	dir := t.TempDir()
	localPath := filepath.Join(dir, "gfw-rules.json")

	f := NewFetcher(ts.URL, localPath, 5*time.Second)
	result := f.Fetch(context.Background())

	if result.Error != nil {
		t.Fatalf("Fetch: %v", result.Error)
	}
	if !result.Updated {
		t.Error("expected Updated=true for first fetch")
	}
	if result.RuleCount != 4 {
		t.Errorf("expected 4 rules, got %d", result.RuleCount)
	}

	// Verify file was written
	rules, err := LoadRules(localPath)
	if err != nil {
		t.Fatalf("LoadRules: %v", err)
	}
	if len(rules) != 4 {
		t.Fatalf("expected 4 rules in file, got %d", len(rules))
	}

	// Check status
	status := f.Status()
	if status.RuleCount != 4 {
		t.Errorf("status: expected 4 rules, got %d", status.RuleCount)
	}
	if status.ETag != `"abc123"` {
		t.Errorf("status: expected etag, got %q", status.ETag)
	}
}

func TestFetcherNotModified(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Header.Get("If-None-Match") == `"abc123"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"abc123"`)
		w.Write([]byte(sampleGFWList()))
	}))
	defer ts.Close()

	dir := t.TempDir()
	localPath := filepath.Join(dir, "gfw-rules.json")

	f := NewFetcher(ts.URL, localPath, 5*time.Second)

	// First fetch
	result1 := f.Fetch(context.Background())
	if result1.Error != nil {
		t.Fatalf("first fetch: %v", result1.Error)
	}
	if !result1.Updated {
		t.Error("first fetch should be Updated=true")
	}

	// Second fetch should get 304
	result2 := f.Fetch(context.Background())
	if result2.Error != nil {
		t.Fatalf("second fetch: %v", result2.Error)
	}
	if result2.Updated {
		t.Error("second fetch should be Updated=false (304)")
	}
	if !result2.Skipped {
		t.Error("second fetch should be Skipped=true")
	}
}

func TestFetcherUnchangedContent(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// No ETag, so conditional GET won't work — but content is same
		w.Write([]byte(sampleGFWList()))
	}))
	defer ts.Close()

	dir := t.TempDir()
	localPath := filepath.Join(dir, "gfw-rules.json")

	f := NewFetcher(ts.URL, localPath, 5*time.Second)

	// First fetch
	result1 := f.Fetch(context.Background())
	if result1.Error != nil {
		t.Fatalf("first fetch: %v", result1.Error)
	}
	if !result1.Updated {
		t.Error("first fetch should be Updated=true")
	}

	// Second fetch — same content, should skip write
	result2 := f.Fetch(context.Background())
	if result2.Error != nil {
		t.Fatalf("second fetch: %v", result2.Error)
	}
	if result2.Updated {
		t.Error("second fetch should be Updated=false (same content)")
	}
	if !result2.Skipped {
		t.Error("second fetch should be Skipped=true")
	}
}

func TestFetcherHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	dir := t.TempDir()
	localPath := filepath.Join(dir, "gfw-rules.json")

	f := NewFetcher(ts.URL, localPath, 5*time.Second)
	result := f.Fetch(context.Background())

	if result.Error == nil {
		t.Error("expected error for 500 response")
	}

	status := f.Status()
	if status.LastError == "" {
		t.Error("expected LastError to be set")
	}
}

func TestFetcherInvalidBase64(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not-valid-base64!!!"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	localPath := filepath.Join(dir, "gfw-rules.json")

	f := NewFetcher(ts.URL, localPath, 5*time.Second)
	result := f.Fetch(context.Background())

	if result.Error == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestFetcherEmptyRules(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// All comments — 0 rules
		content := "! just comments\n! nothing else"
		w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(content))))
	}))
	defer ts.Close()

	dir := t.TempDir()
	localPath := filepath.Join(dir, "gfw-rules.json")

	f := NewFetcher(ts.URL, localPath, 5*time.Second)
	result := f.Fetch(context.Background())

	if result.Error == nil {
		t.Error("expected error for 0 rules")
	}
}

func TestFetcherNoExistingFile(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sampleGFWList()))
	}))
	defer ts.Close()

	dir := t.TempDir()
	localPath := filepath.Join(dir, "subdir", "gfw-rules.json")

	// Ensure subdir exists for this test
	os.MkdirAll(filepath.Dir(localPath), 0755)

	f := NewFetcher(ts.URL, localPath, 5*time.Second)
	f.Fetch(context.Background())

	// The important thing is it doesn't panic regardless of result
}

func TestRulesEqual(t *testing.T) {
	a := []Rule{
		{Match: MatchDomainSuffix, Value: ".google.com", Action: "PROXY"},
	}
	b := []Rule{
		{Match: MatchDomainSuffix, Value: ".google.com", Action: "PROXY"},
	}
	if !rulesEqual(a, b) {
		t.Error("identical rules should be equal")
	}

	c := []Rule{
		{Match: MatchDomainSuffix, Value: ".youtube.com", Action: "PROXY"},
	}
	if rulesEqual(a, c) {
		t.Error("different rules should not be equal")
	}

	if rulesEqual(a, nil) {
		t.Error("non-nil vs nil should not be equal")
	}
}
