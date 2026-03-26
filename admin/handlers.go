package admin

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/yourorg/proxy-pac/gfw"
	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/router"
	"github.com/yourorg/proxy-pac/socks5"
	"github.com/yourorg/proxy-pac/ssh"
	"github.com/yourorg/proxy-pac/status"
)

//go:embed static/*
var staticAssets embed.FS

// Helper functions for external proxy management

// isLocalhost checks if host is a loopback address or name.
func isLocalhost(host string) bool {
	host = strings.ToLower(host)
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

// getPort extracts the port number from an address string.
func getPort(addr string) (int, error) {
	if addr == "" {
		return 0, nil
	}
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, ":") {
			portStr = strings.TrimPrefix(addr, ":")
		} else {
			return 0, err
		}
	}
	return strconv.Atoi(portStr)
}

// validateProxyAddress checks that the external proxy address does not conflict
// with this proxy's own listening ports when pointing to localhost.
func validateProxyAddress(addr string, ourHTTP, ourSOCKS5, ourAdmin string) error {
	// Split address into host and port parts
	var host string
	var port int
	var err error

	// Handle user@host[:port] format
	if at := strings.Index(addr, "@"); at != -1 {
		addr = addr[at+1:] // Skip user@ part
	}

	// Split host:port
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		// No port in address, just host
		host = addr
		port = 0 // Indicates no port specified
	} else {
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid port: %w", err)
		}
	}

	// Only check for conflicts on localhost if we have a port to check
	if !isLocalhost(host) {
		return nil
	}

	// If no port specified, we can't check for specific port conflicts
	// but we should still warn if someone tries to use a well-known port
	// without specifying it explicitly? For now, just allow it.
	if port == 0 {
		return nil
	}

	// Check for conflicts with our own listeners
	httpPort, _ := getPort(ourHTTP)
	socksPort, _ := getPort(ourSOCKS5)
	adminPort, _ := getPort(ourAdmin)
	if port == httpPort || port == socksPort || port == adminPort {
		return fmt.Errorf("proxy address conflicts with this proxy's listener on a loopback interface")
	}
	return nil
}

// validateSSHProxy checks that SSH-specific fields are valid when Kind=="ssh".
func validateSSHProxy(p *pac.ExternalProxy) error {
	if p.Kind != "ssh" {
		return nil
	}
	switch p.AuthMethod {
	case "password":
		if p.Password == "" {
			return fmt.Errorf("password required for SSH password authentication")
		}
	case "key":
		if p.PrivateKeyPath == "" {
			return fmt.Errorf("private_key_path required for SSH key authentication")
		}
	case "":
		return fmt.Errorf("auth_method required for SSH proxy")
	default:
		return fmt.Errorf("invalid auth_method: %s", p.AuthMethod)
	}
	return nil
}

// FullConfig mirrors the main package's Config for reading/writing the YAML.
type FullConfig struct {
	Proxy struct {
		HTTPListen           string              `yaml:"http_listen" json:"http_listen"`
		SOCKS5Listen         string              `yaml:"socks5_listen" json:"socks5_listen"`
		AdminListen          string              `yaml:"admin_listen" json:"admin_listen"`
		HTTPSListen          string              `yaml:"https_listen" json:"https_listen"` // HTTPS proxy listener address (default :8443)
		PACPath              string              `yaml:"pac_path" json:"pac_path"`
		DefaultExternalProxy string              `yaml:"default_external_proxy" json:"default_external_proxy"`
		ExternalProxies      []pac.ExternalProxy `yaml:"external_proxies" json:"external_proxies"`
		TLS                  pac.TLSConfig       `yaml:"tls" json:"tls"` // TLS configuration for HTTPS proxy (certificate, key, CA, client auth)
	} `yaml:"proxy"`
	PAC struct {
		LocalIP        string        `yaml:"local_ip" json:"local_ip"`
		DNSTimeout     time.Duration `yaml:"dns_timeout" json:"dns_timeout"`
		EvalTimeout    time.Duration `yaml:"eval_timeout" json:"eval_timeout"`
		EnableDNSCache bool          `yaml:"enable_dns_cache" json:"enable_dns_cache"`
		EnableURLGlob  bool          `yaml:"enable_url_glob" json:"enable_url_glob"`
		DNSMaxEntries  int           `yaml:"dns_max_entries" json:"dns_max_entries"`
		DNSTTL         time.Duration `yaml:"dns_ttl" json:"dns_ttl"`
		CacheEnabled   bool          `yaml:"cache_enabled" json:"cache_enabled"`
		CacheMaxSize   int           `yaml:"cache_max_size" json:"cache_max_size"`
		CacheTTL       time.Duration `yaml:"cache_ttl" json:"cache_ttl"`
	} `yaml:"pac"`
	GFW struct {
		Disabled         bool          `yaml:"disabled" json:"disabled"`
		SourceURL        string        `yaml:"source_url" json:"source_url"`
		LocalRulesPath   string        `yaml:"local_rules_path" json:"local_rules_path"`
		UserRulesPath    string        `yaml:"user_rules_path" json:"user_rules_path"`
		UpdateInterval   time.Duration `yaml:"update_interval" json:"update_interval"`
		FetchOnStartup   bool          `yaml:"fetch_on_startup" json:"fetch_on_startup"`
		HTTPTimeout      time.Duration `yaml:"http_timeout" json:"http_timeout"`
		CustomDNSServers []string      `yaml:"custom_dns_servers" json:"custom_dns_servers"`
	} `yaml:"gfw"`
}

// Handler provides admin UI and API endpoints.
type Handler struct {
	evaluator            *pac.Evaluator
	pacPath              string
	fetcher              *gfw.Fetcher
	userPath             string
	gfwPath              string
	statusProvider       status.StatusProvider
	configPath           string         // path to config YAML
	router               *router.Router // for proxy registry management
	defaultExternalProxy string         // cached for PAC regeneration
}

// NewHandler creates a new admin handler.
func NewHandler(evaluator *pac.Evaluator, pacPath string, fetcher *gfw.Fetcher, userPath, gfwPath, configPath string, statusProvider status.StatusProvider, router *router.Router, defaultExternalProxy string) *Handler {
	return &Handler{
		evaluator:            evaluator,
		pacPath:              pacPath,
		fetcher:              fetcher,
		userPath:             userPath,
		gfwPath:              gfwPath,
		statusProvider:       statusProvider,
		configPath:           configPath,
		router:               router,
		defaultExternalProxy: defaultExternalProxy,
	}
}

// RegisterRoutes adds admin routes to the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/static/", h.handleStatic)
	mux.HandleFunc("/", h.handleUI)
	mux.HandleFunc("/pac", h.handlePAC)
	mux.HandleFunc("/test", h.handleTest)
	mux.HandleFunc("/status", h.handleStatus)
	// Rules API
	mux.HandleFunc("/api/rules", h.handleRules)
	mux.HandleFunc("/api/rules/", h.handleRuleByID)
	// GFW API
	mux.HandleFunc("/api/gfw/status", h.handleGFWStatus)
	mux.HandleFunc("/api/gfw/fetch", h.handleGWFFetch)
	mux.HandleFunc("/api/gfw/config", h.handleGFWConfig)
	// Proxies API
	mux.HandleFunc("/api/proxies/test", h.handleTestProxy)
	mux.HandleFunc("/api/proxies/", h.handleProxyByID)
	mux.HandleFunc("/api/proxies", h.handleProxies)
	// Config API
	mux.HandleFunc("/api/config", h.handleConfig)
	// Cache API
	mux.HandleFunc("/api/cache", h.handleCacheStats)
	// Status API
	mux.HandleFunc("/api/status", h.handleStatusAPI)
	mux.HandleFunc("/api/connections", h.handleConnections)
}

// handleUI serves the admin UI.
func (h *Handler) handleUI(w http.ResponseWriter, r *http.Request) {
	data, err := staticAssets.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Failed to read embedded UI: %v", err)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

// handlePAC handles GET (read) and POST (save) for the PAC file.
func (h *Handler) handleStatic(w http.ResponseWriter, r *http.Request) {
	filePath := strings.TrimPrefix(r.URL.Path, "/static/")
	if strings.Contains(filePath, "..") {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	data, err := staticAssets.ReadFile("static/" + filePath)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Write(data)
}

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

	// Validate that PAC contains __PAC_RULES__ with valid JSON
	startMarker := "var __PAC_RULES__ = "
	start := strings.Index(content, startMarker)
	if start < 0 {
		http.Error(w, "PAC must contain __PAC_RULES__ structured rules", http.StatusUnprocessableEntity)
		return
	}
	start += len(startMarker)
	end := strings.Index(content[start:], ";")
	if end < 0 {
		http.Error(w, "__PAC_RULES__ not properly terminated", http.StatusUnprocessableEntity)
		return
	}
	end += start
	jsonStr := content[start:end]
	var rf struct {
		Version int           `json:"version"`
		Rules   []interface{} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &rf); err != nil {
		http.Error(w, fmt.Sprintf("Invalid __PAC_RULES__ JSON: %v", err), http.StatusUnprocessableEntity)
		return
	}
	if rf.Version != 1 {
		http.Error(w, fmt.Sprintf("Unsupported __PAC_RULES__ version: %d", rf.Version), http.StatusUnprocessableEntity)
		return
	}

	// Atomic write: tmp file + rename
	tmpPath := h.pacPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0o644); err != nil {
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

// handleStatus returns basic status information.
func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
	})
}

// handleStatusAPI returns detailed server status.
func (h *Handler) handleStatusAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	var serverStatus status.ServerStatus
	if h.statusProvider != nil {
		serverStatus = h.statusProvider.Status()
	}
	// Add SSH tunnel status if available
	if mgr := h.router.GetTunnelManager(); mgr != nil {
		serverStatus.SSHTunnels = mgr.GetTunnelsStatus()
	}
	// Add cache stats if cache is enabled
	if h.evaluator != nil {
		hits, misses, size, maxSize, hitRate := h.evaluator.CacheStats()
		if maxSize > 0 { // cache is enabled
			// We'll add to a separate field; serverStatus may not have Cache field
			// Let's create a wrapper to include cache
			type statusWithCache struct {
				status.ServerStatus
				Cache map[string]interface{} `json:"cache,omitempty"`
			}
			swc := statusWithCache{
				ServerStatus: serverStatus,
				Cache: map[string]interface{}{
					"hits":     hits,
					"misses":   misses,
					"size":     size,
					"max_size": maxSize,
					"hit_rate": hitRate,
				},
			}
			json.NewEncoder(w).Encode(swc)
			return
		}
	}
	json.NewEncoder(w).Encode(serverStatus)
}

// handleConnections returns active connections.
func (h *Handler) handleConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if h.statusProvider == nil {
		json.NewEncoder(w).Encode([]status.Connection{})
		return
	}
	json.NewEncoder(w).Encode(h.statusProvider.Connections())
}

// handleCacheStats returns cache statistics.
func (h *Handler) handleCacheStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	hits, misses, size, maxSize, hitRate := h.evaluator.CacheStats()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"hits":     hits,
		"misses":   misses,
		"size":     size,
		"max_size": maxSize,
		"hit_rate": hitRate,
	})
}

// handleRules handles GET (list all) and POST (create user rule).
func (h *Handler) handleRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleListRules(w, r)
	case http.MethodPost:
		h.handleCreateRule(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleListRules returns all rules (user + GFW).
func (h *Handler) handleListRules(w http.ResponseWriter, r *http.Request) {
	var allRules []gfw.Rule

	// User rules
	if h.userPath != "" {
		userRules, err := gfw.LoadRules(h.userPath)
		if err == nil {
			// Normalize: rules without source are treated as user rules (for backward compatibility)
			for i := range userRules {
				if userRules[i].Source == "" {
					userRules[i].Source = "user"
				}
			}
			allRules = append(allRules, userRules...)
		} else {
			log.Printf("[Admin] Failed to load user rules: %v", err)
		}
	}

	// GFW rules
	if h.gfwPath != "" {
		gfwRules, err := gfw.LoadRules(h.gfwPath)
		if err == nil {
			// Normalize: rules without source are treated as GFW rules
			for i := range gfwRules {
				if gfwRules[i].Source == "" {
					gfwRules[i].Source = "gfw"
				}
			}
			allRules = append(allRules, gfwRules...)
		} else {
			log.Printf("[Admin] Failed to load GFW rules: %v", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(allRules)
}

// handleCreateRule creates a new user rule.
func (h *Handler) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	if h.userPath == "" {
		http.Error(w, "User rules path not configured", http.StatusServiceUnavailable)
		return
	}

	var rule gfw.Rule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	mgr := gfw.NewRuleManager(h.userPath)
	created, err := mgr.Create(rule)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.regeneratePAC()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

// handleRuleByID routes PUT/DELETE/... to /api/rules/:id
func (h *Handler) handleRuleByID(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path: /api/rules/{id} or /api/rules/{id}/toggle
	path := strings.TrimPrefix(r.URL.Path, "/api/rules/")
	parts := strings.SplitN(path, "/", 2)
	id := parts[0]

	if id == "reorder" {
		h.handleReorderRules(w, r)
		return
	}

	isToggle := len(parts) == 2 && parts[1] == "toggle"

	switch {
	case r.Method == http.MethodPut && isToggle:
		h.handleToggleRule(w, r, id)
	case r.Method == http.MethodPut:
		h.handleUpdateRule(w, r, id)
	case r.Method == http.MethodDelete:
		h.handleDeleteRule(w, r, id)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleUpdateRule updates an existing user rule.
func (h *Handler) handleUpdateRule(w http.ResponseWriter, r *http.Request, id string) {
	if h.userPath == "" {
		http.Error(w, "User rules path not configured", http.StatusServiceUnavailable)
		return
	}

	var rule gfw.Rule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	mgr := gfw.NewRuleManager(h.userPath)
	updated, err := mgr.Update(id, rule)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	h.regeneratePAC()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// handleDeleteRule deletes a user rule.
func (h *Handler) handleDeleteRule(w http.ResponseWriter, r *http.Request, id string) {
	if h.userPath == "" {
		http.Error(w, "User rules path not configured", http.StatusServiceUnavailable)
		return
	}

	mgr := gfw.NewRuleManager(h.userPath)
	if err := mgr.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	h.regeneratePAC()

	w.WriteHeader(http.StatusNoContent)
}

// handleToggleRule enables/disables a rule.
func (h *Handler) handleToggleRule(w http.ResponseWriter, r *http.Request, id string) {
	if h.userPath == "" {
		http.Error(w, "User rules path not configured", http.StatusServiceUnavailable)
		return
	}

	mgr := gfw.NewRuleManager(h.userPath)
	toggled, err := mgr.Toggle(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	h.regeneratePAC()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toggled)
}

// handleReorderRules reorders user rules.
func (h *Handler) handleReorderRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.userPath == "" {
		http.Error(w, "User rules path not configured", http.StatusServiceUnavailable)
		return
	}

	var ids []string
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		http.Error(w, "Invalid JSON: expected array of rule IDs", http.StatusBadRequest)
		return
	}

	mgr := gfw.NewRuleManager(h.userPath)
	if err := mgr.Reorder(ids); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.regeneratePAC()

	w.WriteHeader(http.StatusOK)
}

// handleGFWStatus returns the current GFW fetcher status.
func (h *Handler) handleGFWStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.fetcher == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"configured": false,
		})
		return
	}

	status := h.fetcher.Status()
	resp := map[string]interface{}{
		"configured": true,
		"lastFetch":  status.LastFetch,
		"ruleCount":  status.RuleCount,
	}
	if status.LastError != "" {
		resp["lastError"] = status.LastError
	}
	if status.ETag != "" {
		resp["etag"] = status.ETag
	}

	// Determine enabled flag from config
	enabled := true // default (disabled false)
	if h.configPath != "" {
		data, err := os.ReadFile(h.configPath)
		if err == nil {
			var cfg FullConfig
			if err := yaml.Unmarshal(data, &cfg); err == nil {
				enabled = !cfg.GFW.Disabled
			}
		}
	}
	resp["enabled"] = enabled

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleGWFFetch triggers a manual GFW list fetch.
func (h *Handler) handleGWFFetch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.fetcher == nil {
		http.Error(w, "GFW fetcher not configured", http.StatusServiceUnavailable)
		return
	}
	if h.fetcher.IsDisabled() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "GFW fetching disabled"})
		return
	}

	result := h.fetcher.Fetch(context.Background())
	resp := map[string]interface{}{
		"updated":   result.Updated,
		"ruleCount": result.RuleCount,
		"skipped":   result.Skipped,
	}
	if result.Error != nil {
		resp["error"] = result.Error.Error()
	}

	if result.Updated {
		h.regeneratePAC()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleGFWConfig handles GET/PUT for GFW configuration.
func (h *Handler) handleGFWConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleGetGFWConfig(w, r)
	case http.MethodPut:
		h.handlePutGFWConfig(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetGFWConfig returns current GFW configuration.
func (h *Handler) handleGetGFWConfig(w http.ResponseWriter, r *http.Request) {
	if h.fetcher == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"configured": false,
		})
		return
	}

	// Load full config to get Disabled and other persisted values
	data, err := os.ReadFile(h.configPath)
	var cfg FullConfig
	if err == nil {
		yaml.Unmarshal(data, &cfg)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"configured":     true,
		"enabled":        !cfg.GFW.Disabled, // send enabled for UI convenience
		"sourceURL":      h.fetcher.SourceURL,
		"localRulesPath": h.gfwPath,
		"userRulesPath":  h.userPath,
		"fetchOnStartup": cfg.GFW.FetchOnStartup,
		"updateInterval": cfg.GFW.UpdateInterval,
		// could also include other fields if needed
	})
}

// handlePutGFWConfig updates GFW configuration and persists to config file.
func (h *Handler) handlePutGFWConfig(w http.ResponseWriter, r *http.Request) {
	if h.fetcher == nil {
		http.Error(w, "GFW fetcher not configured", http.StatusServiceUnavailable)
		return
	}

	// Load current config to get current disabled state
	data, err := os.ReadFile(h.configPath)
	if err != nil {
		http.Error(w, "Failed to read config", http.StatusInternalServerError)
		return
	}
	var cfg FullConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		http.Error(w, "Failed to parse config", http.StatusInternalServerError)
		return
	}

	var req struct {
		SourceURL string `json:"sourceURL"`
		Disabled  *bool  `json:"disabled"`
		Enabled   *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Merge changes
	changed := false
	if req.SourceURL != "" && req.SourceURL != h.fetcher.SourceURL {
		h.fetcher.SourceURL = req.SourceURL
		cfg.GFW.SourceURL = req.SourceURL
		changed = true
	}

	currentDisabled := cfg.GFW.Disabled
	newDisabled := currentDisabled
	if req.Enabled != nil {
		newDisabled = !*req.Enabled
	}
	if req.Disabled != nil {
		newDisabled = *req.Disabled
	}

	if newDisabled != currentDisabled {
		h.fetcher.SetDisabled(newDisabled)
		cfg.GFW.Disabled = newDisabled
		changed = true
	}

	if !changed {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	// Persist updated config
	out, err := yaml.Marshal(&cfg)
	if err != nil {
		http.Error(w, "Failed to marshal config", http.StatusInternalServerError)
		return
	}
	tmpPath := h.configPath + ".tmp"
	if err := os.WriteFile(tmpPath, out, 0o644); err != nil {
		http.Error(w, "Failed to write config tmp", http.StatusInternalServerError)
		return
	}
	if err := os.Rename(tmpPath, h.configPath); err != nil {
		os.Remove(tmpPath)
		http.Error(w, "Failed to rename config", http.StatusInternalServerError)
		return
	}

	// Regenerate PAC immediately
	h.regeneratePAC()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// regeneratePAC merges user and GFW rules and writes the PAC file.
func (h *Handler) regeneratePAC() {
	if h.userPath == "" || h.gfwPath == "" || h.pacPath == "" {
		return
	}
	// Read config to get EnableURLGlob and GFW enabled state
	enableURLGlob := false
	enabled := true // default
	if h.configPath != "" {
		data, err := os.ReadFile(h.configPath)
		if err == nil {
			var cfg FullConfig
			if err := yaml.Unmarshal(data, &cfg); err == nil {
				enableURLGlob = cfg.PAC.EnableURLGlob
				enabled = !cfg.GFW.Disabled
			}
		}
	}
	if err := gfw.RegeneratePAC(h.userPath, h.gfwPath, h.pacPath, h.defaultExternalProxy, enableURLGlob, enabled); err != nil {
		log.Printf("[Admin] PAC regeneration failed: %v", err)
	}
}

// writeFullConfig writes the FullConfig to h.configPath atomically.
func (h *Handler) writeFullConfig(cfg FullConfig) error {
	out, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmpPath := h.configPath + ".tmp"
	if err := os.WriteFile(tmpPath, out, 0o644); err != nil {
		return fmt.Errorf("write config tmp: %w", err)
	}
	if err := os.Rename(tmpPath, h.configPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

// handleProxies dispatches GET (list) and POST (create) on /api/proxies.
func (h *Handler) handleProxies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleListProxies(w, r)
	case http.MethodPost:
		h.handleCreateProxy(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleListProxies returns the list of external proxies without passwords.
func (h *Handler) handleListProxies(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(h.configPath)
	if err != nil {
		log.Printf("[Admin] Failed to read config for listing proxies: %v", err)
		http.Error(w, "Cannot read config", http.StatusInternalServerError)
		return
	}
	var cfg FullConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Printf("[Admin] Failed to parse config for listing proxies: %v", err)
		http.Error(w, "Cannot parse config", http.StatusInternalServerError)
		return
	}
	proxies := make([]map[string]interface{}, len(cfg.Proxy.ExternalProxies))
	for i, p := range cfg.Proxy.ExternalProxies {
		proxies[i] = map[string]interface{}{
			"id":       p.ID,
			"address":  p.Address,
			"kind":     p.Kind,
			"has_auth": (p.Username != "" || p.Password != ""),
		}
	}
	// Add keepalive fields to each proxy entry
	for i, p := range cfg.Proxy.ExternalProxies {
		keepaliveInterval := ""
		if p.Keepalive.Interval > 0 {
			keepaliveInterval = p.Keepalive.Interval.String()
		}
		proxies[i]["keepalive_enabled"] = p.Keepalive.Enabled
		proxies[i]["keepalive_interval"] = keepaliveInterval
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(proxies)
}

// handleCreateProxy adds a new external proxy.
func (h *Handler) handleCreateProxy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID                 string `json:"id"`
		Address            string `json:"address"`
		Username           string `json:"username"`
		Password           string `json:"password"`
		Kind               string `json:"kind"`
		SSHPort            int    `json:"ssh_port"`
		AuthMethod         string `json:"auth_method"`
		PrivateKeyPath     string `json:"private_key_path"`
		HostKeyCheck       string `json:"host_key_check"`
		HostKeyFingerprint string `json:"host_key_fingerprint"`
		KeepaliveEnabled   *bool  `json:"keepalive_enabled"`
		KeepaliveInterval  string `json:"keepalive_interval"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if req.Address == "" {
		http.Error(w, "address is required", http.StatusBadRequest)
		return
	}
	// Build proxy
	newProxy := pac.ExternalProxy{
		ID:                 req.ID,
		Address:            req.Address,
		Username:           req.Username,
		Password:           req.Password,
		Kind:               req.Kind,
		SSHPort:            req.SSHPort,
		AuthMethod:         req.AuthMethod,
		PrivateKeyPath:     req.PrivateKeyPath,
		HostKeyCheck:       req.HostKeyCheck,
		HostKeyFingerprint: req.HostKeyFingerprint,
	}
	// Apply keepalive if any field provided
	if req.KeepaliveEnabled != nil || req.KeepaliveInterval != "" {
		newProxy.Keepalive = pac.KeepaliveConfig{}
		if req.KeepaliveEnabled != nil {
			newProxy.Keepalive.Enabled = *req.KeepaliveEnabled
		}
		if req.KeepaliveInterval != "" {
			if d, err := time.ParseDuration(req.KeepaliveInterval); err == nil {
				newProxy.Keepalive.Interval = d
			}
		}
	}
	// Ensure default if enabled and interval zero
	if newProxy.Keepalive.Enabled && newProxy.Keepalive.Interval == 0 {
		newProxy.Keepalive.Interval = 30 * time.Second
	}
	// Validate address format and loop prevention
	status := h.statusProvider.Status()
	if err := validateProxyAddress(newProxy.Address, status.HTTPListen, status.SOCKS5Listen, status.AdminListen); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Reserved ID check
	switch newProxy.ID {
	case "DIRECT", "DEFAULT":
		http.Error(w, "reserved proxy id", http.StatusBadRequest)
		return
	}
	// Read config
	data, err := os.ReadFile(h.configPath)
	if err != nil {
		log.Printf("[Admin] Failed to read config for create proxy: %v", err)
		http.Error(w, "Cannot read config", http.StatusInternalServerError)
		return
	}
	var cfg FullConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Printf("[Admin] Failed to parse config for create proxy: %v", err)
		http.Error(w, "Cannot parse config", http.StatusInternalServerError)
		return
	}
	// Duplicate ID check
	for _, p := range cfg.Proxy.ExternalProxies {
		if p.ID == newProxy.ID {
			http.Error(w, "duplicate proxy id", http.StatusConflict)
			return
		}
	}
	// Validate SSH-specific fields if present
	if err := validateSSHProxy(&newProxy); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg.Proxy.ExternalProxies = append(cfg.Proxy.ExternalProxies, newProxy)
	if err := h.writeFullConfig(cfg); err != nil {
		log.Printf("[Admin] Failed to write config after create proxy: %v", err)
		http.Error(w, "Failed to save config", http.StatusInternalServerError)
		return
	}
	// Update router registry
	h.router.SetProxyRegistry(cfg.Proxy.ExternalProxies)
	// Respond
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	keepaliveInterval := ""
	if newProxy.Keepalive.Interval > 0 {
		keepaliveInterval = newProxy.Keepalive.Interval.String()
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":                 newProxy.ID,
		"address":            newProxy.Address,
		"has_auth":           (newProxy.Username != "" || newProxy.Password != ""),
		"keepalive_enabled":  newProxy.Keepalive.Enabled,
		"keepalive_interval": keepaliveInterval,
	})
}

// handleProxyByID handles /api/proxies/:id (PUT, DELETE) and /api/proxies/:id/test (POST test).
func (h *Handler) handleProxyByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/proxies/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		http.Error(w, "proxy id required", http.StatusBadRequest)
		return
	}
	id := parts[0]
	if len(parts) > 1 && parts[1] == "test" && r.Method == http.MethodPost {
		h.handleTestProxyByID(w, r, id)
		return
	}
	switch r.Method {
	case http.MethodPut:
		h.handleUpdateProxy(w, r, id)
	case http.MethodDelete:
		h.handleDeleteProxy(w, r, id)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleTestProxyByID tests connectivity to a saved proxy.
func (h *Handler) handleTestProxyByID(w http.ResponseWriter, r *http.Request, id string) {
	// Read config
	data, readErr := os.ReadFile(h.configPath)
	if readErr != nil {
		http.Error(w, "Cannot read config", http.StatusInternalServerError)
		return
	}
	var cfg FullConfig
	if unmarshalErr := yaml.Unmarshal(data, &cfg); unmarshalErr != nil {
		http.Error(w, "Cannot parse config", http.StatusInternalServerError)
		return
	}
	// Find proxy
	var proxy pac.ExternalProxy
	found := false
	for _, p := range cfg.Proxy.ExternalProxies {
		if p.ID == id {
			proxy = p
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "Proxy not found", http.StatusNotFound)
		return
	}
	// Test connection
	timeout := 5 * time.Second
	var conn net.Conn
	var dialErr error
	if proxy.Kind == "ssh" {
		mgr := h.router.GetTunnelManager()
		if mgr == nil {
			http.Error(w, "SSH tunnel manager not configured", http.StatusInternalServerError)
			return
		}
		conn, dialErr = mgr.Dial(proxy.ID, "1.1.1.1:80", timeout)
	} else {
		// SOCKS5 test
		conn, dialErr = socks5.DialWithAuth("tcp", proxy.Address, &socks5.Auth{Username: proxy.Username, Password: proxy.Password}, timeout)
	}
	if dialErr != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": dialErr.Error()})
		return
	}
	conn.Close()
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// handleTestProxy tests an arbitrary unsaved proxy configuration.
// Supports both SOCKS5 (default) and SSH (when kind: "ssh") proxies.
func (h *Handler) handleTestProxy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address            string `json:"address"`
		Username           string `json:"username"`
		Password           string `json:"password"`
		Kind               string `json:"kind"`
		AuthMethod         string `json:"auth_method"`
		PrivateKeyPath     string `json:"private_key_path"`
		SSHPort            int    `json:"ssh_port"`
		HostKeyCheck       string `json:"host_key_check"`
		HostKeyFingerprint string `json:"host_key_fingerprint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Address == "" {
		http.Error(w, "address required", http.StatusBadRequest)
		return
	}

	// Default to SOCKS5 if kind not specified (backward compatibility)
	if req.Kind == "" {
		req.Kind = "socks5"
	}

	timeout := 5 * time.Second

	// SSH proxy test
	if req.Kind == "ssh" {
		// Validate required SSH fields
		if req.AuthMethod == "" {
			http.Error(w, "auth_method required for SSH proxy", http.StatusBadRequest)
			return
		}
		if req.AuthMethod == "key" && req.PrivateKeyPath == "" {
			http.Error(w, "private_key_path required for key auth", http.StatusBadRequest)
			return
		}
		if req.AuthMethod == "password" && req.Password == "" {
			http.Error(w, "password required for password auth", http.StatusBadRequest)
			return
		}

		// Normalize host_key_check (default: strict)
		hostKeyCheck := req.HostKeyCheck
		if hostKeyCheck != "ignore" {
			hostKeyCheck = "strict"
		}

		// Build temporary proxy config for test
		proxy := pac.ExternalProxy{
			ID:                 "test-unsaved",
			Kind:               "ssh",
			Address:            req.Address,
			Username:           req.Username,
			Password:           req.Password,
			SSHPort:            req.SSHPort,
			AuthMethod:         req.AuthMethod,
			PrivateKeyPath:     req.PrivateKeyPath,
			HostKeyCheck:       hostKeyCheck,
			HostKeyFingerprint: req.HostKeyFingerprint,
		}

		// Create temporary tunnel (not managed by tunnel manager)
		tunnel := ssh.NewTunnel(proxy)
		defer tunnel.Close()

		// Test connection by dialing a neutral destination
		conn, err := tunnel.Dial("1.1.1.1:80", timeout)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		conn.Close()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
		return
	}

	// SOCKS5 proxy test (original behavior)
	conn, err := socks5.DialWithAuth("tcp", req.Address, &socks5.Auth{Username: req.Username, Password: req.Password}, timeout)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	conn.Close()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// handleUpdateProxy updates an existing external proxy (partial update).
func (h *Handler) handleUpdateProxy(w http.ResponseWriter, r *http.Request, id string) {
	// Read config
	data, err := os.ReadFile(h.configPath)
	if err != nil {
		log.Printf("[Admin] Failed to read config for update proxy: %v", err)
		http.Error(w, "Cannot read config", http.StatusInternalServerError)
		return
	}
	var cfg FullConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Printf("[Admin] Failed to parse config for update proxy: %v", err)
		http.Error(w, "Cannot parse config", http.StatusInternalServerError)
		return
	}
	// Find proxy
	idx := -1
	for i, p := range cfg.Proxy.ExternalProxies {
		if p.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		http.Error(w, "Proxy not found", http.StatusNotFound)
		return
	}
	var upd struct {
		Address            string `json:"address"`
		Username           string `json:"username"`
		Password           string `json:"password"`
		Kind               string `json:"kind"`
		SSHPort            int    `json:"ssh_port"`
		AuthMethod         string `json:"auth_method"`
		PrivateKeyPath     string `json:"private_key_path"`
		HostKeyCheck       string `json:"host_key_check"`
		HostKeyFingerprint string `json:"host_key_fingerprint"`
		KeepaliveEnabled   *bool  `json:"keepalive_enabled"`
		KeepaliveInterval  string `json:"keepalive_interval"`
	}
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	proxy := cfg.Proxy.ExternalProxies[idx]
	if upd.Address != "" {
		status := h.statusProvider.Status()
		if err := validateProxyAddress(upd.Address, status.HTTPListen, status.SOCKS5Listen, status.AdminListen); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		proxy.Address = upd.Address
	}
	if upd.Username != "" {
		proxy.Username = upd.Username
	}
	if upd.Password != "" {
		proxy.Password = upd.Password
	}
	// SSH fields
	if upd.Kind != "" {
		proxy.Kind = upd.Kind
	}
	if upd.SSHPort != 0 {
		proxy.SSHPort = upd.SSHPort
	}
	if upd.AuthMethod != "" {
		proxy.AuthMethod = upd.AuthMethod
	}
	if upd.PrivateKeyPath != "" {
		proxy.PrivateKeyPath = upd.PrivateKeyPath
	}
	if upd.HostKeyCheck != "" {
		proxy.HostKeyCheck = upd.HostKeyCheck
	}
	if upd.HostKeyFingerprint != "" {
		proxy.HostKeyFingerprint = upd.HostKeyFingerprint
	}
	// Apply keepalive updates if provided
	if upd.KeepaliveEnabled != nil {
		proxy.Keepalive.Enabled = *upd.KeepaliveEnabled
	}
	if upd.KeepaliveInterval != "" {
		if d, err := time.ParseDuration(upd.KeepaliveInterval); err == nil {
			proxy.Keepalive.Interval = d
		}
	}
	// Ensure keepalive defaults if needed
	if proxy.Keepalive.Enabled && proxy.Keepalive.Interval == 0 {
		proxy.Keepalive.Interval = 30 * time.Second
	}
	// Validate SSH configuration
	if err := validateSSHProxy(&proxy); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg.Proxy.ExternalProxies[idx] = proxy
	if err := h.writeFullConfig(cfg); err != nil {
		log.Printf("[Admin] Failed to write config after update proxy: %v", err)
		http.Error(w, "Failed to save config", http.StatusInternalServerError)
		return
	}
	h.router.SetProxyRegistry(cfg.Proxy.ExternalProxies)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(proxy)
}

// handleDeleteProxy removes an external proxy by ID.
func (h *Handler) handleDeleteProxy(w http.ResponseWriter, r *http.Request, id string) {
	data, err := os.ReadFile(h.configPath)
	if err != nil {
		log.Printf("[Admin] Failed to read config for delete proxy: %v", err)
		http.Error(w, "Cannot read config", http.StatusInternalServerError)
		return
	}
	var cfg FullConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Printf("[Admin] Failed to parse config for delete proxy: %v", err)
		http.Error(w, "Cannot parse config", http.StatusInternalServerError)
		return
	}
	newProxies := make([]pac.ExternalProxy, 0, len(cfg.Proxy.ExternalProxies))
	found := false
	for _, p := range cfg.Proxy.ExternalProxies {
		if p.ID == id {
			found = true
			continue
		}
		newProxies = append(newProxies, p)
	}
	if !found {
		http.Error(w, "Proxy not found", http.StatusNotFound)
		return
	}
	cfg.Proxy.ExternalProxies = newProxies
	if err := h.writeFullConfig(cfg); err != nil {
		log.Printf("[Admin] Failed to write config after delete proxy: %v", err)
		http.Error(w, "Failed to save config", http.StatusInternalServerError)
		return
	}
	h.router.SetProxyRegistry(cfg.Proxy.ExternalProxies)
	w.WriteHeader(http.StatusNoContent)
}

// handleConfig dispatches GET and PUT on /api/config.
func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleGetConfig(w, r)
	case http.MethodPut:
		h.handlePutConfig(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetConfig returns current proxy configuration (listeners, pac_path, default_external_proxy).
func (h *Handler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	status := h.statusProvider.Status()
	cfg := struct {
		HTTPListen           string `json:"http_listen"`
		SOCKS5Listen         string `json:"socks5_listen"`
		AdminListen          string `json:"admin_listen"`
		HTTPSListen          string `json:"https_listen"`
		PACPath              string `json:"pac_path"`
		DefaultExternalProxy string `json:"default_external_proxy"`
		PAC                  struct {
			LocalIP        string `json:"local_ip"`
			DNSTimeout     string `json:"dns_timeout"`
			EvalTimeout    string `json:"eval_timeout"`
			EnableDNSCache bool   `json:"enable_dns_cache"`
			DNSMaxEntries  int    `json:"dns_max_entries"`
			DNSTTL         string `json:"dns_ttl"`
			CacheEnabled   bool   `json:"cache_enabled"`
			CacheMaxSize   int    `json:"cache_max_size"`
			CacheTTL       string `json:"cache_ttl"`
			EnableURLGlob  bool   `json:"enable_url_glob"`
		} `json:"pac"`
		TLS struct {
			CertificateFile string `json:"certificate_file"`
			KeyFile         string `json:"key_file"`
			CAFile          string `json:"ca_file"`
			ClientAuth      string `json:"client_auth"`
		} `json:"tls"`
	}{
		HTTPListen:   status.HTTPListen,
		SOCKS5Listen: status.SOCKS5Listen,
		AdminListen:  status.AdminListen,
		HTTPSListen:  status.HTTPSListen, // Use status if available, else from config
	}
	// Read config file to get all settings
	data, err := os.ReadFile(h.configPath)
	if err == nil {
		var full FullConfig
		if err := yaml.Unmarshal(data, &full); err == nil {
			cfg.PACPath = full.Proxy.PACPath
			cfg.DefaultExternalProxy = full.Proxy.DefaultExternalProxy
			cfg.HTTPSListen = full.Proxy.HTTPSListen
			cfg.PAC.LocalIP = full.PAC.LocalIP
			cfg.PAC.DNSTimeout = full.PAC.DNSTimeout.String()
			cfg.PAC.EvalTimeout = full.PAC.EvalTimeout.String()
			cfg.PAC.EnableDNSCache = full.PAC.EnableDNSCache
			cfg.PAC.DNSMaxEntries = full.PAC.DNSMaxEntries
			cfg.PAC.DNSTTL = full.PAC.DNSTTL.String()
			cfg.PAC.CacheEnabled = full.PAC.CacheEnabled
			cfg.PAC.CacheMaxSize = full.PAC.CacheMaxSize
			cfg.PAC.CacheTTL = full.PAC.CacheTTL.String()
			cfg.PAC.EnableURLGlob = full.PAC.EnableURLGlob
			cfg.TLS.CertificateFile = full.Proxy.TLS.CertificateFile
			cfg.TLS.KeyFile = full.Proxy.TLS.KeyFile
			cfg.TLS.CAFile = full.Proxy.TLS.CAFile
			cfg.TLS.ClientAuth = full.Proxy.TLS.ClientAuth
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

// handlePutConfig updates the configuration (partial update).
func (h *Handler) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var upd struct {
		HTTPListen           string `json:"http_listen"`
		SOCKS5Listen         string `json:"socks5_listen"`
		AdminListen          string `json:"admin_listen"`
		HTTPSListen          string `json:"https_listen"`
		PACPath              string `json:"pac_path"`
		TUNDevice            string `json:"tun_device"`
		DefaultExternalProxy string `json:"default_external_proxy"`
		EnableURLGlob        bool   `json:"enable_url_glob"`
		PAC                  struct {
			LocalIP        string `json:"local_ip"`
			DNSTimeout     string `json:"dns_timeout"` // duration string
			EvalTimeout    string `json:"eval_timeout"`
			EnableDNSCache bool   `json:"enable_dns_cache"`
			DNSMaxEntries  int    `json:"dns_max_entries"`
			DNSTTL         string `json:"dns_ttl"`
			CacheEnabled   bool   `json:"cache_enabled"`
			CacheMaxSize   int    `json:"cache_max_size"`
			CacheTTL       string `json:"cache_ttl"`
		} `json:"pac"`
		TLS struct {
			CertificateFile string `json:"certificate_file"`
			KeyFile         string `json:"key_file"`
			CAFile          string `json:"ca_file"`
			ClientAuth      string `json:"client_auth"`
		} `json:"tls"`
	}
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	// Load full config
	data, err := os.ReadFile(h.configPath)
	if err != nil {
		log.Printf("[Admin] Failed to read config for put: %v", err)
		http.Error(w, "Cannot read config", http.StatusInternalServerError)
		return
	}
	var cfg FullConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Printf("[Admin] Failed to parse config: %v", err)
		http.Error(w, "Cannot parse config", http.StatusInternalServerError)
		return
	}
	restartRequired := false
	needRegen := false
	if upd.HTTPListen != "" {
		cfg.Proxy.HTTPListen = upd.HTTPListen
		restartRequired = true
	}
	if upd.SOCKS5Listen != "" {
		cfg.Proxy.SOCKS5Listen = upd.SOCKS5Listen
		restartRequired = true
	}
	if upd.AdminListen != "" {
		cfg.Proxy.AdminListen = upd.AdminListen
		restartRequired = true
	}
	if upd.HTTPSListen != "" {
		cfg.Proxy.HTTPSListen = upd.HTTPSListen
		restartRequired = true
	}
	if upd.PACPath != "" {
		cfg.Proxy.PACPath = upd.PACPath
	}
	if upd.DefaultExternalProxy != "" && upd.DefaultExternalProxy != cfg.Proxy.DefaultExternalProxy {
		cfg.Proxy.DefaultExternalProxy = upd.DefaultExternalProxy
		h.router.SetDefaultProxyID(upd.DefaultExternalProxy)
		h.defaultExternalProxy = upd.DefaultExternalProxy
		needRegen = true
	}
	// TLS updates (only if provided)
	if upd.TLS.CertificateFile != "" {
		cfg.Proxy.TLS.CertificateFile = upd.TLS.CertificateFile
		restartRequired = true
	}
	if upd.TLS.KeyFile != "" {
		cfg.Proxy.TLS.KeyFile = upd.TLS.KeyFile
		restartRequired = true
	}
	if upd.TLS.CAFile != "" {
		cfg.Proxy.TLS.CAFile = upd.TLS.CAFile
		restartRequired = true
	}
	if upd.TLS.ClientAuth != "" {
		cfg.Proxy.TLS.ClientAuth = upd.TLS.ClientAuth
		restartRequired = true
	}
	// Handle EnableURLGlob change
	if upd.EnableURLGlob != cfg.PAC.EnableURLGlob {
		cfg.PAC.EnableURLGlob = upd.EnableURLGlob
		needRegen = true
	}

	// Handle other PAC fields
	if upd.PAC.LocalIP != "" && upd.PAC.LocalIP != cfg.PAC.LocalIP {
		cfg.PAC.LocalIP = upd.PAC.LocalIP
		needRegen = true
	}
	if upd.PAC.DNSTimeout != "" {
		if d, err := time.ParseDuration(upd.PAC.DNSTimeout); err == nil {
			if d != cfg.PAC.DNSTimeout {
				cfg.PAC.DNSTimeout = d
				needRegen = true
			}
		}
	}
	if upd.PAC.EvalTimeout != "" {
		if d, err := time.ParseDuration(upd.PAC.EvalTimeout); err == nil {
			if d != cfg.PAC.EvalTimeout {
				cfg.PAC.EvalTimeout = d
				needRegen = true
			}
		}
	}
	if upd.PAC.EnableDNSCache != cfg.PAC.EnableDNSCache {
		cfg.PAC.EnableDNSCache = upd.PAC.EnableDNSCache
		needRegen = true
	}
	if upd.PAC.DNSMaxEntries != 0 && upd.PAC.DNSMaxEntries != cfg.PAC.DNSMaxEntries {
		cfg.PAC.DNSMaxEntries = upd.PAC.DNSMaxEntries
		needRegen = true
	}
	if upd.PAC.DNSTTL != "" {
		if d, err := time.ParseDuration(upd.PAC.DNSTTL); err == nil {
			if d != cfg.PAC.DNSTTL {
				cfg.PAC.DNSTTL = d
				needRegen = true
			}
		}
	}
	if upd.PAC.CacheEnabled != cfg.PAC.CacheEnabled {
		cfg.PAC.CacheEnabled = upd.PAC.CacheEnabled
		needRegen = true
	}
	if upd.PAC.CacheMaxSize != 0 && upd.PAC.CacheMaxSize != cfg.PAC.CacheMaxSize {
		cfg.PAC.CacheMaxSize = upd.PAC.CacheMaxSize
		needRegen = true
	}
	if upd.PAC.CacheTTL != "" {
		if d, err := time.ParseDuration(upd.PAC.CacheTTL); err == nil {
			if d != cfg.PAC.CacheTTL {
				cfg.PAC.CacheTTL = d
				needRegen = true
			}
		}
	}

	// Write updated config
	if err := h.writeFullConfig(cfg); err != nil {
		log.Printf("[Admin] Failed to write config: %v", err)
		http.Error(w, "Failed to save config", http.StatusInternalServerError)
		return
	}
	if needRegen {
		h.regeneratePAC()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":           "ok",
		"restart_required": restartRequired,
	})
}
