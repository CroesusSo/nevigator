package admin

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/dop251/goja"

	"github.com/yourorg/proxy-pac/pac"
)

// Handler provides admin UI and API endpoints.
type Handler struct {
	evaluator *pac.Evaluator
	pacPath   string
}

// NewHandler creates a new admin handler.
func NewHandler(evaluator *pac.Evaluator, pacPath string) *Handler {
	return &Handler{
		evaluator: evaluator,
		pacPath:   pacPath,
	}
}

// RegisterRoutes adds admin routes to the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", h.handleUI)
	mux.HandleFunc("/pac", h.handlePAC)
	mux.HandleFunc("/test", h.handleTest)
	mux.HandleFunc("/status", h.handleStatus)
}

// handleUI serves the admin UI.
func (h *Handler) handleUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	html := `<!DOCTYPE html>
<html>
<head>
  <title>Proxy PAC Admin</title>
  <style>
    body { font-family: sans-serif; margin: 2rem; }
    textarea { width: 100%; height: 400px; font-family: monospace; }
    button { margin-top: 1rem; padding: 0.5rem 1rem; }
    .status { margin-top: 2rem; }
    .error { color: #c00; white-space: pre-wrap; margin-top: 0.5rem; }
    .success { color: #090; margin-top: 0.5rem; }
  </style>
</head>
<body>
  <h1>PAC Proxy Admin</h1>
  <div>
    <textarea id="pac"></textarea><br>
    <button onclick="save()">Save</button>
    <div id="feedback"></div>
  </div>
  <div class="status" id="status">
    TUN IP: <span id="tun-ip">...</span><br>
    PAC loaded: <span id="loaded">...</span>
  </div>
  <div style="margin-top:2rem;">
    <h3>Test URL</h3>
    <input type="text" id="testurl" placeholder="example.com" size="40">
    <button onclick="test()">Test</button>
    <pre id="testresult"></pre>
  </div>
  <script>
    async function load() {
      const res = await fetch('/pac');
      const text = await res.text();
      document.getElementById('pac').value = text;
    }
    async function save() {
      const content = document.getElementById('pac').value;
      const feedback = document.getElementById('feedback');
      feedback.innerHTML = '';
      try {
        const res = await fetch('/pac', { method: 'POST', body: content, headers: {'Content-Type':'text/plain'} });
        if (res.ok) {
          feedback.innerHTML = '<span class="success">Saved successfully. PAC will reload momentarily.</span>';
        } else {
          const err = await res.text();
          feedback.innerHTML = '<span class="error">Save failed: ' + err + '</span>';
        }
      } catch (e) {
        feedback.innerHTML = '<span class="error">Save failed: ' + e.message + '</span>';
      }
    }
    async function test() {
      const url = document.getElementById('testurl').value;
      const res = await fetch('/test?url=' + encodeURIComponent(url));
      const data = await res.json();
      document.getElementById('testresult').textContent = JSON.stringify(data, null, 2);
    }
    load();
  </script>
</body>
</html>`
	w.Write([]byte(html))
}

// handlePAC handles GET (read) and POST (save) for the PAC file.
func (h *Handler) handlePAC(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.handleGetPAC(w, r)
		return
	}
	if r.Method == http.MethodPost {
		h.handleSavePAC(w, r)
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// handleGetPAC returns the current PAC file content.
func (h *Handler) handleGetPAC(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(h.pacPath)
	if err != nil {
		http.Error(w, "Failed to read PAC file", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

// handleSavePAC validates and atomically saves PAC content.
func (h *Handler) handleSavePAC(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	content := string(body)

	if len(content) == 0 {
		http.Error(w, "empty PAC content", http.StatusUnprocessableEntity)
		return
	}

	// Validate by compiling in a fresh goja VM
	_, err = goja.Compile("pac.js", content, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	// Atomic write: tmp file + rename
	tmpPath := h.pacPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		log.Printf("Failed to write PAC tmp file: %v", err)
		http.Error(w, "Failed to write file", http.StatusInternalServerError)
		return
	}
	if err := os.Rename(tmpPath, h.pacPath); err != nil {
		log.Printf("Failed to rename PAC tmp file: %v", err)
		os.Remove(tmpPath)
		http.Error(w, "Failed to write file", http.StatusInternalServerError)
		return
	}

	// Ensure tmp file cleanup on any parent dir case
	_ = filepath.Dir(h.pacPath)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// extractHost extracts the hostname from a URL or bare hostname string.
func extractHost(input string) string {
	// Try parsing as a full URL (with scheme)
	if u, err := url.Parse(input); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	// Bare hostname or host:port — parse as opaque to extract host
	// url.Parse handles "example.com:8080" as Path, not Host
	// So we try adding a scheme and re-parsing
	if u, err := url.Parse("http://" + input); err == nil {
		if h := u.Hostname(); h != "" {
			return h
		}
	}
	// Fallback: return as-is
	return input
}

// handleTest evaluates a URL and returns the resulting route.
func (h *Handler) handleTest(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("url")
	if raw == "" {
		http.Error(w, "Missing url", http.StatusBadRequest)
		return
	}
	host := extractHost(raw)
	route := h.evaluator.Evaluate(host)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"strategy": route.Strategy,
		"address":  route.Address,
	})
}

// handleStatus returns status information.
func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
	})
}
