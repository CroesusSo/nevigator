package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/proxy-pac/gfw"
	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/router"
)

// newTestEvaluator builds an evaluator with an empty rule set, the state
// before the GFW list has ever been fetched.
func newTestEvaluator(t *testing.T) *pac.Evaluator {
	t.Helper()
	dir := t.TempDir()
	pacPath := filepath.Join(dir, "proxy.pac")
	if err := os.WriteFile(pacPath, []byte(gfw.GeneratePAC(nil, nil, gfw.FWMeta{})), 0644); err != nil {
		t.Fatal(err)
	}
	ev, err := pac.NewEvaluator(pac.EvaluatorConfig{
		PACPath:       pacPath,
		EvalTimeout:   time.Second,
		FallbackRoute: pac.Route{Strategy: "DIRECT"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ev.Close() })
	return ev
}

// TestNewHandlerPinsGFWSourceRoute proves that the handler pins the route of
// the GFW list source host.
//
// The list is fetched before any rule for that host exists, so without the
// pin the first fetch leaves the host directly and fails on a censored
// network.
func TestNewHandlerPinsGFWSourceRoute(t *testing.T) {
	ev := newTestEvaluator(t)
	const sourceHost = "raw.githubusercontent.com"

	fetcher := gfw.NewFetcher("https://"+sourceHost+"/gfwlist/gfwlist/master/gfwlist.txt",
		filepath.Join(t.TempDir(), "gfw-rules.json"), 10*time.Second)

	rt, err := router.NewRouter(pac.RouterConfig{DialTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	rt.SetDefaultProxyID("upstream-1")

	NewHandler(ev, "", fetcher, "", "", "", &mockStatusProvider{}, rt, "upstream-1")

	route := ev.Evaluate(sourceHost)
	if route.Strategy != "DEFAULT" {
		t.Errorf("source host route = %q, want DEFAULT", route.Strategy)
	}
}

// TestGFWSourceRouteFollowsSourceURL proves that changing the source URL
// moves the pinned route to the new host and releases the old one.
func TestGFWSourceRouteFollowsSourceURL(t *testing.T) {
	ev := newTestEvaluator(t)
	const oldHost = "raw.githubusercontent.com"
	const newHost = "list.example.org"

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("gfw:\n  source_url: \"https://"+oldHost+"/list.txt\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	fetcher := gfw.NewFetcher("https://"+oldHost+"/list.txt", filepath.Join(dir, "gfw-rules.json"), 10*time.Second)
	rt, err := router.NewRouter(pac.RouterConfig{DialTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	rt.SetDefaultProxyID("upstream-1")

	h := NewHandler(ev, filepath.Join(dir, "proxy.pac"), fetcher, "", "", configPath, &mockStatusProvider{}, rt, "upstream-1")

	body := `{"sourceURL":"https://` + newHost + `/list.txt"}`
	req := httptest.NewRequest(http.MethodPut, "/api/gfw/config", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.handleGFWConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}

	if got := ev.Evaluate(newHost); got.Strategy != "DEFAULT" {
		t.Errorf("new source host route = %q, want DEFAULT", got.Strategy)
	}
	if got := ev.Evaluate(oldHost); got.Strategy != "DIRECT" {
		t.Errorf("old source host route = %q, want DIRECT; the pin was not released", got.Strategy)
	}
}

// TestManualFetchUsesTheFetcherClient proves that a manual fetch goes through
// the fetcher's client, so it takes the same path as the startup fetch.
//
// The client is the one that tunnels through the local SOCKS5 listener, so
// the manual fetch must not bypass it.
func TestManualFetchUsesTheFetcherClient(t *testing.T) {
	ev := newTestEvaluator(t)

	// A server that must not be reached: the request has to go through the
	// fetcher's transport.
	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("reached directly"))
	}))
	defer direct.Close()

	dir := t.TempDir()
	fetcher := gfw.NewFetcher(direct.URL, filepath.Join(dir, "gfw-rules.json"), 5*time.Second)
	// A transport that always fails stands in for the tunnel.
	fetcher.HTTPClient = &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("tunnel failed")
		}),
		Timeout: 5 * time.Second,
	}

	rt, err := router.NewRouter(pac.RouterConfig{DialTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(ev, filepath.Join(dir, "proxy.pac"), fetcher, "", "", "", &mockStatusProvider{}, rt, "")

	req := httptest.NewRequest(http.MethodPost, "/api/gfw/fetch", nil)
	w := httptest.NewRecorder()
	h.handleGWFFetch(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusOK)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if _, ok := resp["error"]; !ok {
		t.Errorf("response = %v, want an error: the fetch bypassed the fetcher's client", resp)
	}
}

// roundTripperFunc adapts a function to http.RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
