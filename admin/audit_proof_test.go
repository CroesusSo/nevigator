//go:build audit
// +build audit

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/router"
	"github.com/yourorg/proxy-pac/status"
)

// Proof 5: the admin GET /api/proxies/:id endpoint returns the plaintext
// password of an external proxy.
//
// handleListProxies deliberately omits passwords ("has_auth" only) and
// handleGetConfig deliberately reports "has_password" only, but
// handleGetProxyByID marshals the whole pac.ExternalProxy struct, whose
// Password field carries a json:"password" tag.
//
// Expected: the endpoint does not disclose the stored credential.
// Actual: the plaintext password is returned to the caller.
func TestAuditAdminProxyDetailHidesPassword(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "proxy:\n  http_listen: \":8080\"\n  socks5_listen: \":1080\"\n" +
		"  external_proxies:\n" +
		"    - id: \"up\"\n      address: \"127.0.0.1:1081\"\n      username: \"alice\"\n      password: \"s3cret\"\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	rt, err := router.NewRouter(pac.RouterConfig{})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{
		statusProvider: &mockStatusProvider{serverStatus: status.ServerStatus{}},
		configPath:     path,
		router:         rt,
	}

	w := httptest.NewRecorder()
	h.handleProxyByID(w, httptest.NewRequest(http.MethodGet, "/api/proxies/up", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET = %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "s3cret") {
		var m map[string]any
		json.Unmarshal(w.Body.Bytes(), &m)
		t.Fatalf("BUG: GET /api/proxies/up disclosed the stored password in cleartext: %v", m)
	}
}
