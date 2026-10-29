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

// TestAuditAdminProxyUpdateHidesPassword proves that the PUT
// /api/proxies/:id response does not disclose the stored password.
//
// handleUpdateProxy encodes the whole pac.ExternalProxy struct back to the
// caller, so the credential is disclosed on the write path as well as on
// the read path.
//
// Expected: the response does not contain the stored password.
// Actual: the plaintext password is returned.
func TestAuditAdminProxyUpdateHidesPassword(t *testing.T) {
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

	body := `{"address":"127.0.0.1:1082"}`
	w := httptest.NewRecorder()
	h.handleProxyByID(w, httptest.NewRequest(http.MethodPut, "/api/proxies/up", strings.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT = %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "s3cret") {
		var m map[string]any
		json.Unmarshal(w.Body.Bytes(), &m)
		t.Fatalf("BUG: PUT /api/proxies/up disclosed the stored password in cleartext: %v", m)
	}
}
