package admin

import (
	"encoding/json"
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
	"github.com/yourorg/proxy-pac/status"
	"gopkg.in/yaml.v3"
)

func yamlUnmarshalForTest(b []byte, v any) error {
	return yaml.Unmarshal(b, v)
}

// emptyHandler builds a handler with no rules, no proxies and no connections,
// i.e. every collection it can report is empty.
func emptyHandler(t *testing.T) (*Handler, string) {
	t.Helper()
	dir := t.TempDir()
	pacPath := filepath.Join(dir, "proxy.pac")
	if err := os.WriteFile(pacPath, []byte(gfw.GeneratePAC(nil, nil, gfw.FWMeta{})), 0o644); err != nil {
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
	rt, err := router.NewRouter(pac.RouterConfig{})
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("proxy:\n  http_listen: \":8080\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &Handler{
		statusProvider: &mockStatusProvider{serverStatus: status.ServerStatus{}},
		configPath:     cfgPath,
		router:         rt,
		evaluator:      ev,
	}, cfgPath
}

// Collection endpoints must return an empty JSON array, not null. A null
// breaks any client that iterates the response.
func TestEmptyCollectionsReturnArraysNotNull(t *testing.T) {
	h, _ := emptyHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	for _, path := range []string{
		"/api/connections", "/api/rules", "/api/proxies",
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		body := strings.TrimSpace(w.Body.String())
		if body == "null" {
			t.Errorf("%s returned null; want []", path)
			continue
		}
		var v []any
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Errorf("%s did not return a JSON array: %s", path, body)
			continue
		}
		if len(v) != 0 {
			t.Errorf("%s returned %d items, want 0", path, len(v))
		}
	}
}

// The TCP keepalive block must round-trip through the proxy API.
func TestProxyKeepaliveTCPRoundTrip(t *testing.T) {
	h, cfgPath := emptyHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Create with TCP tuning.
	body := `{"id":"hk","kind":"ssh","address":"u@h:22","auth_method":"password",` +
		`"password":"pw","keepalive_enabled":true,"keepalive_interval":"25s",` +
		`"keepalive_tcp":{"enabled":true,"idle":"45s","interval":"12s","count":4}}`
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/proxies", strings.NewReader(body)))
	if w.Code != http.StatusCreated {
		t.Fatalf("POST = %d: %s", w.Code, w.Body.String())
	}

	// Persisted to disk.
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "keepalive") || !strings.Contains(string(data), "tcp") {
		t.Errorf("tcp block not persisted:\n%s", data)
	}
	var cfg FullConfig
	if err := yamlUnmarshalForTest(data, &cfg); err != nil {
		t.Fatal(err)
	}
	p0 := cfg.Proxy.ExternalProxies[0]
	if p0.Keepalive.TCP == nil {
		t.Fatalf("keepalive.tcp not persisted: %+v", p0.Keepalive)
	}
	if p0.Keepalive.TCP.Enabled != true || p0.Keepalive.TCP.Idle.String() != "45s" ||
		p0.Keepalive.TCP.Interval.String() != "12s" || p0.Keepalive.TCP.Count != 4 {
		t.Errorf("tcp values wrong: %+v", p0.Keepalive.TCP)
	}

	// Listed by the collection endpoint with the effective values.
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/proxies", nil))
	var list []map[string]any
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(list))
	}
	ka, ok := list[0]["keepalive_tcp"].(map[string]any)
	if !ok {
		t.Fatalf("keepalive_tcp missing from list response: %v", list[0])
	}
	if ka["enabled"] != true || ka["idle"] != "45s" || ka["interval"] != "12s" || ka["count"].(float64) != 4 {
		t.Errorf("keepalive_tcp in list = %+v", ka)
	}
}

// A proxy with no TCP tuning reports it as disabled rather than omitting it.
func TestProxyWithoutTCPReportsDisabled(t *testing.T) {
	h, _ := emptyHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"id":"hk","kind":"ssh","address":"u@h:22","auth_method":"password","password":"pw"}`
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/proxies", strings.NewReader(body)))
	if w.Code != http.StatusCreated {
		t.Fatalf("POST = %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/proxies", nil))
	var list []map[string]any
	json.Unmarshal(w.Body.Bytes(), &list)
	ka, ok := list[0]["keepalive_tcp"].(map[string]any)
	if !ok {
		t.Fatalf("keepalive_tcp missing: %v", list[0])
	}
	if ka["enabled"] != false {
		t.Errorf("expected enabled=false, got %v", ka["enabled"])
	}
}

// Invalid TCP tuning must be rejected without corrupting the stored config.
func TestProxyRejectsInvalidKeepaliveTCP(t *testing.T) {
	h, cfgPath := emptyHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	for _, bad := range []string{
		`{"id":"a","kind":"ssh","address":"u@h:22","auth_method":"password","password":"p","keepalive_tcp":{"enabled":true,"idle":"zzz"}}`,
		`{"id":"a","kind":"ssh","address":"u@h:22","auth_method":"password","password":"p","keepalive_tcp":{"enabled":true,"count":-1}}`,
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/proxies", strings.NewReader(bad)))
		if w.Code != http.StatusBadRequest {
			t.Errorf("POST invalid keepalive_tcp = %d, want 400", w.Code)
		}
	}
	data, _ := os.ReadFile(cfgPath)
	if strings.Contains(string(data), "tcp") {
		t.Errorf("rejected write still modified the file:\n%s", data)
	}
}
