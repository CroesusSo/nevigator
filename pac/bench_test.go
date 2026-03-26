package pac

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/gfw"
)

func generateLargeNativePAC(numRules int) (string, error) {
	rules := make([]gfw.Rule, 0, numRules)
	for i := 0; i < numRules; i++ {
		value := fmt.Sprintf(".example%d.com", i)
		rules = append(rules, gfw.Rule{
			ID:      fmt.Sprintf("id%d", i),
			Source:  "test",
			Enabled: true,
			Match:   gfw.MatchDomainSuffix,
			Value:   value,
			Action:  "PROXY",
			Route:   "SOCKS5 127.0.0.1:1080",
		})
	}
	rf := gfw.RulesFile{Version: 1, Rules: rules}
	jsonData, _ := json.Marshal(rf)
	pac := "var __PAC_RULES__ = " + string(jsonData) + ";\nfunction FindProxyForURL(url, host) { return 'DIRECT'; }"
	return pac, nil
}

// BenchmarkEvaluator_Native_Uncached benchmarks native engine evaluation without cache.
func BenchmarkEvaluator_Native_Uncached(b *testing.B) {
	pac, err := generateLargeNativePAC(4100)
	if err != nil {
		b.Fatalf("failed to generate PAC: %v", err)
	}
	tmpFile, err := os.CreateTemp("", "bench_native_*.pac")
	if err != nil {
		b.Fatalf("temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write([]byte(pac)); err != nil {
		b.Fatalf("write pac: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		b.Fatalf("close: %v", err)
	}

	cfg := EvaluatorConfig{
		PACPath:       tmpFile.Name(),
		LocalIP:       "127.0.0.1",
		DNSTimeout:    1 * time.Second,
		EvalTimeout:   5 * time.Second,
		FallbackRoute: Route{Strategy: "DIRECT"},
		CacheEnabled:  false,
	}
	e, err := NewEvaluator(cfg)
	if err != nil {
		b.Fatalf("NewEvaluator: %v", err)
	}
	defer e.Close()

	host := "www.example0.com"
	// Warm up
	e.Evaluate(host)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Evaluate(host)
	}
}

// BenchmarkEvaluator_Native_Cached benchmarks native engine evaluation with hot cache.
func BenchmarkEvaluator_Native_Cached(b *testing.B) {
	pac, err := generateLargeNativePAC(4100)
	if err != nil {
		b.Fatalf("failed to generate PAC: %v", err)
	}
	tmpFile, err := os.CreateTemp("", "bench_native_cache_*.pac")
	if err != nil {
		b.Fatalf("temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write([]byte(pac)); err != nil {
		b.Fatalf("write pac: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		b.Fatalf("close: %v", err)
	}

	cfg := EvaluatorConfig{
		PACPath:       tmpFile.Name(),
		LocalIP:       "127.0.0.1",
		DNSTimeout:    1 * time.Second,
		EvalTimeout:   5 * time.Second,
		FallbackRoute: Route{Strategy: "DIRECT"},
		CacheEnabled:  true,
		CacheMaxSize:  10000,
		CacheTTL:      1 * time.Hour,
	}
	e, err := NewEvaluator(cfg)
	if err != nil {
		b.Fatalf("NewEvaluator: %v", err)
	}
	defer e.Close()

	host := "www.example0.com"
	// Warm up cache: first evaluation miss and populate, second hits
	e.Evaluate(host)
	e.Evaluate(host)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Evaluate(host)
	}
}
