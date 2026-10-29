package pac

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/gfw"
)

// newFallbackEvaluator builds an evaluator over a PAC file with the given
// rules, and a global fallback of DIRECT.
func newFallbackEvaluator(t *testing.T, rules []gfw.Rule, fallbacks map[string]Route) *Evaluator {
	t.Helper()
	dir := t.TempDir()
	pacPath := filepath.Join(dir, "proxy.pac")
	content := gfw.GeneratePAC(rules, nil, gfw.FWMeta{})
	if err := os.WriteFile(pacPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	ev, err := NewEvaluator(EvaluatorConfig{
		PACPath:       pacPath,
		EvalTimeout:   time.Second,
		FallbackRoute: Route{Strategy: "DIRECT"},
		HostFallbacks: fallbacks,
		CacheEnabled:  true,
		CacheMaxSize:  100,
		CacheTTL:      time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ev.Close() })
	return ev
}

// TestHostFallbackAppliesWhenNoRuleMatches proves that a pinned host takes
// its pinned route when no rule matches it.
func TestHostFallbackAppliesWhenNoRuleMatches(t *testing.T) {
	ev := newFallbackEvaluator(t, nil, map[string]Route{
		"raw.githubusercontent.com": {Strategy: "DEFAULT"},
	})

	if got := ev.Evaluate("raw.githubusercontent.com"); got.Strategy != "DEFAULT" {
		t.Errorf("route = %q, want DEFAULT", got.Strategy)
	}
}

// TestHostFallbackDoesNotOverrideExplicitRule proves that a pinned host still
// takes an explicit rule when one matches it.
//
// The pin is a fallback, not an override: a user who routes the host
// explicitly must have that decision honoured.
func TestHostFallbackDoesNotOverrideExplicitRule(t *testing.T) {
	rules := []gfw.Rule{
		{ID: "r1", Source: "user", Enabled: true, Match: gfw.MatchExact, Value: "raw.githubusercontent.com", Action: "PROXY", Route: "DIRECT"},
	}
	ev := newFallbackEvaluator(t, rules, map[string]Route{
		"raw.githubusercontent.com": {Strategy: "DEFAULT"},
	})

	if got := ev.Evaluate("raw.githubusercontent.com"); got.Strategy != "DIRECT" {
		t.Errorf("route = %q, want DIRECT: the explicit rule must win over the pin", got.Strategy)
	}
}

// TestHostFallbackAppliesToThatHostOnly proves that a pin does not leak onto
// other hosts.
//
// The pin is for one host: the source of the GFW list. A neighbouring host
// must keep the global route, or the pin would silently redirect traffic that
// was never meant to go through the upstream.
func TestHostFallbackAppliesToThatHostOnly(t *testing.T) {
	ev := newFallbackEvaluator(t, nil, map[string]Route{
		"raw.githubusercontent.com": {Strategy: "DEFAULT"},
	})

	if got := ev.Evaluate("raw.githubusercontent.com"); got.Strategy != "DEFAULT" {
		t.Errorf("route = %q, want DEFAULT", got.Strategy)
	}
	if got := ev.Evaluate("example.com"); got.Strategy != "DIRECT" {
		t.Errorf("route for an unrelated host = %q, want DIRECT", got.Strategy)
	}
}

// TestSetHostFallbackInvalidatesCache proves that a host pinned after it was
// already evaluated takes the new route.
//
// Without the invalidation the cached evaluation would keep returning the
// old route until the cache entry expired.
func TestSetHostFallbackInvalidatesCache(t *testing.T) {
	ev := newFallbackEvaluator(t, nil, nil)

	if got := ev.Evaluate("raw.githubusercontent.com"); got.Strategy != "DIRECT" {
		t.Fatalf("route before pinning = %q, want DIRECT", got.Strategy)
	}
	ev.SetHostFallback("raw.githubusercontent.com", Route{Strategy: "DEFAULT"})
	if got := ev.Evaluate("raw.githubusercontent.com"); got.Strategy != "DEFAULT" {
		t.Errorf("route after pinning = %q, want DEFAULT", got.Strategy)
	}
}

// TestRemoveHostFallbackReleasesThePin proves that a removed pin lets the
// host fall back to the global route again.
func TestRemoveHostFallbackReleasesThePin(t *testing.T) {
	ev := newFallbackEvaluator(t, nil, map[string]Route{
		"raw.githubusercontent.com": {Strategy: "DEFAULT"},
	})

	ev.RemoveHostFallback("raw.githubusercontent.com")
	if got := ev.Evaluate("raw.githubusercontent.com"); got.Strategy != "DIRECT" {
		t.Errorf("route = %q, want DIRECT", got.Strategy)
	}
}

// TestHostFallbackNormalizesHost proves that a pinned host is matched
// regardless of case and of a trailing port.
func TestHostFallbackNormalizesHost(t *testing.T) {
	ev := newFallbackEvaluator(t, nil, map[string]Route{
		"Raw.GitHubusercontent.com": {Strategy: "DEFAULT"},
	})

	for _, host := range []string{"raw.githubusercontent.com", "RAW.GITHUBUSERCONTENT.COM", "raw.githubusercontent.com:443"} {
		if got := ev.Evaluate(host); got.Strategy != "DEFAULT" {
			t.Errorf("route for %q = %q, want DEFAULT", host, got.Strategy)
		}
	}
}

// TestGFWSourceRoute proves that the GFW source host takes the route the GFW
// rules themselves resolve to.
func TestGFWSourceRoute(t *testing.T) {
	if got := GFWSourceRoute("upstream-1"); got.Strategy != "DEFAULT" {
		t.Errorf("route with a default proxy = %q, want DEFAULT", got.Strategy)
	}
	if got := GFWSourceRoute(""); got.Strategy != "DIRECT" {
		t.Errorf("route without a default proxy = %q, want DIRECT", got.Strategy)
	}
}
