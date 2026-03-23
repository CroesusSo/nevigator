package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleTestProxy_SSHValidation tests the validation logic for SSH proxy test requests.
// It does not test actual SSH connection (covered by integration tests).
func TestHandleTestProxy_SSHValidation(t *testing.T) {
	tests := []struct {
		name           string
		jsonBody       string
		wantStatus     int
		wantBodySubstr string
	}{
		{
			name:           "Missing auth_method",
			jsonBody:       `{"address":"example.com","kind":"ssh"}`,
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "auth_method required for SSH proxy",
		},
		{
			name:           "Key auth without private_key_path",
			jsonBody:       `{"address":"example.com","kind":"ssh","auth_method":"key"}`,
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "private_key_path required for key auth",
		},
		{
			name:           "Password auth without password",
			jsonBody:       `{"address":"example.com","kind":"ssh","auth_method":"password"}`,
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "password required for password auth",
		},
	}

	h := &Handler{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/proxies/test", strings.NewReader(tt.jsonBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.handleTestProxy(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			resp := w.Body.String()
			if !strings.Contains(resp, tt.wantBodySubstr) {
				t.Errorf("response = %q, should contain %q", resp, tt.wantBodySubstr)
			}
		})
	}
}

// TestHandleTestProxy_BackwardCompatibility ensures that when kind is omitted or empty,
// the handler treats it as SOCKS5 and does not attempt SSH logic.
// We use an address that will fail connection quickly (connection refused) and expect HTTP 200
// with success=false, not a 400 validation error.
func TestHandleTestProxy_BackwardCompatibility(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/proxies/test", strings.NewReader(`{"address":"127.0.0.1:99999"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h := &Handler{}
	h.handleTestProxy(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (backward compat: missing kind should not cause 400)", w.Code, http.StatusOK)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if success, ok := resp["success"]; !ok || success != false {
		t.Fatalf("expected success=false in response, got %v", resp)
	}
	// Should not have an error about missing auth_method (which would indicate SSH path)
	if errMsg, ok := resp["error"]; ok {
		if s, ok2 := errMsg.(string); ok2 && strings.Contains(s, "auth_method") {
			t.Fatalf("got SSH validation error in response: %v", resp)
		}
	}
}
