package gfw

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// Fetcher downloads and parses gfwlist.txt files.
type Fetcher struct {
	SourceURL  string
	LocalPath  string
	HTTPClient *http.Client
	disabled   int32 // 0=enabled, 1=disabled (atomic)

	mu        sync.Mutex
	lastFetch time.Time
	lastError string
	ruleCount int
	etag      string
	lastMod   string
}

// SourceHost returns the host name of a GFW list source URL. It returns an
// empty string when the URL carries no host.
func SourceHost(sourceURL string) string {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// FetchResult contains the outcome of a fetch attempt.
type FetchResult struct {
	Updated   bool
	RuleCount int
	Skipped   bool
	Error     error
}

// FetchStatus contains current fetcher state.
type FetchStatus struct {
	LastFetch time.Time `json:"lastFetch"`
	RuleCount int       `json:"ruleCount"`
	LastError string    `json:"lastError,omitempty"`
	ETag      string    `json:"etag,omitempty"`
	LastMod   string    `json:"lastMod,omitempty"`
}

// NewFetcher creates a new GFW list fetcher.
func NewFetcher(sourceURL, localPath string, timeout time.Duration) *Fetcher {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Fetcher{
		SourceURL:  sourceURL,
		LocalPath:  localPath,
		HTTPClient: &http.Client{Timeout: timeout},
		disabled:   0,
	}
}

// SetDisabled enables or disables fetching.
func (f *Fetcher) SetDisabled(disabled bool) {
	if disabled {
		atomic.StoreInt32(&f.disabled, 1)
	} else {
		atomic.StoreInt32(&f.disabled, 0)
	}
}

// IsDisabled reports whether fetching is disabled.
func (f *Fetcher) IsDisabled() bool {
	return atomic.LoadInt32(&f.disabled) == 1
}

// Fetch downloads the gfwlist, decodes it, parses rules, and saves if changed.
func (f *Fetcher) Fetch(ctx context.Context) FetchResult {
	if f.IsDisabled() {
		return FetchResult{Error: fmt.Errorf("GFW fetching disabled")}
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	// 1. HTTP GET with conditional headers
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.SourceURL, nil)
	if err != nil {
		return f.recordError(fmt.Errorf("create request: %v", err))
	}
	if f.etag != "" {
		req.Header.Set("If-None-Match", f.etag)
	}
	if f.lastMod != "" {
		req.Header.Set("If-Modified-Since", f.lastMod)
	}

	resp, err := f.HTTPClient.Do(req)
	if err != nil {
		return f.recordError(fmt.Errorf("fetch: %v", err))
	}
	defer resp.Body.Close()

	// 2. Handle 304 Not Modified
	if resp.StatusCode == http.StatusNotModified {
		log.Printf("[GFW] Not modified (304), skipping")
		f.lastFetch = time.Now()
		f.lastError = ""
		return FetchResult{Updated: false, RuleCount: f.ruleCount, Skipped: true}
	}

	if resp.StatusCode != http.StatusOK {
		return f.recordError(fmt.Errorf("unexpected status: %d", resp.StatusCode))
	}

	// 3. Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return f.recordError(fmt.Errorf("read body: %v", err))
	}

	// 4. Base64 decode
	decoded, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		// Try without padding
		decoded, err = base64.RawStdEncoding.DecodeString(string(body))
		if err != nil {
			return f.recordError(fmt.Errorf("base64 decode: %v", err))
		}
	}

	// 5. Parse rules
	rules := ParseGFWList(string(decoded))
	if len(rules) == 0 {
		return f.recordError(fmt.Errorf("parsed 0 rules from gfwlist"))
	}

	// 6. Compare with existing
	existing, _ := LoadRules(f.LocalPath)
	if rulesEqual(existing, rules) {
		log.Printf("[GFW] Rules unchanged (%d), skipping write", len(rules))
		f.updateMeta(resp, len(rules))
		return FetchResult{Updated: false, RuleCount: len(rules), Skipped: true}
	}

	// 7. Write atomically
	if err := SaveRules(f.LocalPath, rules); err != nil {
		return f.recordError(fmt.Errorf("save rules: %v", err))
	}

	f.updateMeta(resp, len(rules))
	log.Printf("[GFW] Updated: %d rules written to %s", len(rules), f.LocalPath)
	return FetchResult{Updated: true, RuleCount: len(rules)}
}

// Status returns the current fetcher state.
func (f *Fetcher) Status() FetchStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return FetchStatus{
		LastFetch: f.lastFetch,
		RuleCount: f.ruleCount,
		LastError: f.lastError,
		ETag:      f.etag,
		LastMod:   f.lastMod,
	}
}

func (f *Fetcher) updateMeta(resp *http.Response, count int) {
	f.lastFetch = time.Now()
	f.ruleCount = count
	f.lastError = ""
	if etag := resp.Header.Get("ETag"); etag != "" {
		f.etag = etag
	}
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		f.lastMod = lm
	}
}

func (f *Fetcher) recordError(err error) FetchResult {
	f.lastError = err.Error()
	f.lastFetch = time.Now()
	log.Printf("[GFW] Fetch error: %v", err)
	return FetchResult{Error: err}
}

// rulesEqual compares two rule slices for semantic equality.
func rulesEqual(a, b []Rule) bool {
	if len(a) != len(b) {
		return false
	}
	// Compare by match type + value + action (ignore ID)
	for i := range a {
		if a[i].Match != b[i].Match || a[i].Value != b[i].Value || a[i].Action != b[i].Action {
			return false
		}
	}
	return true
}
