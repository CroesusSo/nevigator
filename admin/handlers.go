package admin

import (
	"context"
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

	"github.com/dop251/goja"
	"gopkg.in/yaml.v3"

	"github.com/yourorg/proxy-pac/gfw"
	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/router"
	"github.com/yourorg/proxy-pac/socks5"
	"github.com/yourorg/proxy-pac/status"
)

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
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address format: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}
	if !isLocalhost(host) {
		return nil
	}
	httpPort, _ := getPort(ourHTTP)
	socksPort, _ := getPort(ourSOCKS5)
	adminPort, _ := getPort(ourAdmin)
	if port == httpPort || port == socksPort || port == adminPort {
		return fmt.Errorf("proxy address conflicts with this proxy's listener on a loopback interface")
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
		TUNDevice            string              `yaml:"tun_device" json:"tun_device"`
		DefaultExternalProxy string              `yaml:"default_external_proxy" json:"default_external_proxy"`
		ExternalProxies      []pac.ExternalProxy `yaml:"external_proxies" json:"external_proxies"`
		TLS                  pac.TLSConfig       `yaml:"tls" json:"tls"` // TLS configuration for HTTPS proxy (certificate, key, CA, client auth)
	} `yaml:"proxy"`
	PAC struct {
		LocalIP        string        `yaml:"local_ip" json:"local_ip"`
		DNSTimeout     time.Duration `yaml:"dns_timeout" json:"dns_timeout"`
		EvalTimeout    time.Duration `yaml:"eval_timeout" json:"eval_timeout"`
		EnableDNSCache bool          `yaml:"enable_dns_cache" json:"enable_dns_cache"`
		CacheEnabled   bool          `yaml:"cache_enabled" json:"cache_enabled"`
		CacheMaxSize   int           `yaml:"cache_max_size" json:"cache_max_size"`
		CacheTTL       time.Duration `yaml:"cache_ttl" json:"cache_ttl"`
	} `yaml:"pac"`
	GFW struct {
		SourceURL      string        `yaml:"source_url" json:"source_url"`
		LocalRulesPath string        `yaml:"local_rules_path" json:"local_rules_path"`
		UserRulesPath  string        `yaml:"user_rules_path" json:"user_rules_path"`
		UseTUN         bool          `yaml:"use_tun" json:"use_tun"`
		UpdateInterval time.Duration `yaml:"update_interval" json:"update_interval"`
		FetchOnStartup bool          `yaml:"fetch_on_startup" json:"fetch_on_startup"`
		HTTPTimeout    time.Duration `yaml:"http_timeout" json:"http_timeout"`
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
	w.Header().Set("Content-Type", "text/html")
	html := `<!DOCTYPE html>
<html>
<head>
  <title>Proxy PAC Admin</title>
  <style>
    body { font-family: sans-serif; margin: 0; background: #f5f5f5; }
    .header { background: #1a1a2e; color: #fff; padding: 1rem 2rem; }
    .header h1 { margin: 0; font-size: 1.4rem; }
    .tabs { display: flex; background: #16213e; padding: 0 2rem; }
    .tab { padding: 0.8rem 1.5rem; color: #aaa; cursor: pointer; border-bottom: 3px solid transparent; }
    .tab.active { color: #fff; border-bottom-color: #e94560; }
    .tab:hover { color: #fff; }
    .content { padding: 2rem; max-width: 1200px; margin: 0 auto; }
    .panel { display: none; }
    .panel.active { display: block; }
    table { width: 100%; border-collapse: collapse; background: #fff; border-radius: 8px; overflow: hidden; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }
    th, td { padding: 0.6rem 1rem; text-align: left; border-bottom: 1px solid #eee; }
    th { background: #f8f8f8; font-weight: 600; font-size: 0.85rem; color: #666; }
    td { font-size: 0.9rem; }
    .btn { padding: 0.4rem 0.8rem; border: none; border-radius: 4px; cursor: pointer; font-size: 0.85rem; }
    .btn-primary { background: #e94560; color: #fff; }
    .btn-primary:hover { background: #c73e54; }
    .btn-secondary { background: #eee; color: #333; }
    .btn-secondary:hover { background: #ddd; }
    .btn-danger { background: #dc3545; color: #fff; }
    .btn-sm { padding: 0.25rem 0.5rem; font-size: 0.8rem; }
    input, select { padding: 0.4rem 0.6rem; border: 1px solid #ddd; border-radius: 4px; font-size: 0.9rem; }
    .form-row { display: flex; gap: 0.5rem; align-items: center; margin-bottom: 1rem; flex-wrap: wrap; }
    .form-row label { font-size: 0.85rem; color: #666; min-width: 80px; }
    .form-row input, .form-row select { flex: 1; min-width: 150px; }
    .badge { padding: 0.15rem 0.5rem; border-radius: 10px; font-size: 0.75rem; font-weight: 600; }
    .badge-user { background: #e3f2fd; color: #1565c0; }
    .badge-gfw { background: #fff3e0; color: #e65100; }
    .toggle { cursor: pointer; }
    .toggle.on { color: #090; }
    .toggle.off { color: #c00; }
    .card { background: #fff; border-radius: 8px; padding: 1.5rem; box-shadow: 0 1px 3px rgba(0,0,0,0.1); margin-bottom: 1rem; }
    .card h3 { margin: 0 0 1rem 0; font-size: 1rem; }
    .status-line { font-size: 0.85rem; color: #666; margin-top: 0.5rem; }
    .success { color: #090; }
    .error { color: #c00; }
    pre { background: #f8f8f8; padding: 1rem; border-radius: 4px; overflow: auto; }
    .drag-handle { cursor: move; color: #ccc; }
    .status-dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin-right: 6px; }
    .status-dot.up { background: #090; }
    .status-dot.down { background: #c00; }
    .kv { display: flex; padding: 0.4rem 0; border-bottom: 1px solid #f0f0f0; }
    .kv:last-child { border-bottom: none; }
    .kv-key { width: 160px; color: #666; font-size: 0.85rem; flex-shrink: 0; }
    .kv-val { font-size: 0.9rem; }
    .badge-tun { background: #e8f5e9; color: #2e7d32; }
    .badge-socks5 { background: #fff3e0; color: #e65100; }
    .badge-http { background: #e3f2fd; color: #1565c0; }
    .badge-direct { background: #f5f5f5; color: #666; }
    .badge-host_glob { background: #e3f2fd; color: #1565c0; }
    .badge-url_glob { background: #e3f2fd; color: #1565c0; }
    .badge-domain_suffix { background: #e3f2fd; color: #1565c0; }
    .badge-exact { background: #e3f2fd; color: #1565c0; }
    .pattern { cursor: pointer; }
    .pattern:hover { text-decoration: underline; }
    .invalid { border-color: #dc3545 !important; }
    .edit-controls button { background: none; border: none; cursor: pointer; font-size: 1.2rem; margin-left: 0.25rem; }
    .edit-controls .save { color: #090; }
    .edit-controls .cancel { color: #c00; }
    .gfw-section { background: #f5f5f5; padding: 1rem; border-radius: 8px; margin-top: 1rem; position: relative; height: 400px; overflow-y: auto; }
    .gfw-section table { margin-top: 0.5rem; }
    .gfw-section th { font-weight: 600; color: #666; }
    .gfw-section tbody { position: relative; }
    .gfw-row { background: #fafafa; }
    .hint { font-size: 0.85rem; color: #666; margin-bottom: 1rem; }
    .count-badge { font-weight: bold; }
  </style>
</head>
<body>
  <div class="header"><h1>Proxy PAC Admin</h1></div>
  <div class="tabs">
    <div class="tab active" onclick="switchTab('summary')">Summary</div>
    <div class="tab" onclick="switchTab('connections')">Connections</div>
    <div class="tab" onclick="switchTab('rules')">Rules</div>
    <div class="tab" onclick="switchTab('gfw')">GFW List</div>
     <div class="tab" onclick="switchTab('test')">Test URL</div>
     <div class="tab" onclick="switchTab('proxies')">Proxies</div>
     <div class="tab" onclick="switchTab('settings')">Settings</div>
  </div>
  <div class="content">

    <!-- Summary Tab -->
    <div id="panel-summary" class="panel active">
      <div class="card">
        <h3>Listening Addresses</h3>
        <div class="kv"><span class="kv-key">HTTP Proxy</span><span class="kv-val" id="s-http">-</span></div>
        <div class="kv"><span class="kv-key">SOCKS5 Proxy</span><span class="kv-val" id="s-socks5">-</span></div>
        <div class="kv"><span class="kv-key">Admin UI</span><span class="kv-val" id="s-admin">-</span></div>
      </div>
      <div class="card">
        <h3>Outgoing</h3>
        <div class="kv"><span class="kv-key">TUN Device</span><span class="kv-val" id="s-tun">-</span></div>
      </div>
      <div class="card">
        <h3>Connections</h3>
        <div class="kv"><span class="kv-key">Active</span><span class="kv-val" id="s-active">-</span></div>
        <div class="kv"><span class="kv-key">Total (since start)</span><span class="kv-val" id="s-total">-</span></div>
      </div>
    </div>

    <!-- Connections Tab -->
    <div id="panel-connections" class="panel">
      <div class="card">
        <h3>Active Connections <span id="conn-count" style="color:#999;font-weight:normal;font-size:0.85rem;"></span></h3>
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>Client</th>
              <th>Destination</th>
              <th>Route</th>
              <th>Protocol</th>
              <th>Duration</th>
            </tr>
          </thead>
          <tbody id="conn-body"></tbody>
        </table>
        <div id="conn-empty" style="padding:2rem;text-align:center;color:#999;display:none;">No active connections</div>
      </div>
    </div>

     <!-- Rules Tab -->
     <div id="panel-rules" class="panel">
       <div class="card">
         <h3>Add Rule</h3>
         <div class="form-row">
           <label>Match</label>
           <select id="new-match">
             <option value="domain_suffix">Domain Suffix</option>
             <option value="host_glob">Host Glob</option>
             <option value="url_glob">URL Glob</option>
             <option value="exact">Exact</option>
           </select>
           <label>Pattern</label>
           <input id="new-value" placeholder=".example.com">
           <label>Route</label>
           <select id="new-route">
             <option value="DIRECT">DIRECT</option>
             <option value="TUN">TUN</option>
             <option value="SOCKS5 127.0.0.1:1080">SOCKS5 (default)</option>
           </select>
           <button class="btn btn-primary" onclick="addRule()">Add</button>
         </div>
       </div>
       <p class="hint">Rules are evaluated top-to-bottom. User rules always take precedence over GFW rules.</p>
       <h3>User Rules</h3>
       <table>
         <thead>
           <tr>
             <th width="30">#</th>
             <th width="60">Source</th>
             <th width="50">On/Off</th>
             <th width="120">Match Type <span title="domain_suffix: match domain suffixes; host_glob: * and ? wildcards for hostnames; url_glob: * and ? wildcards for full URLs; exact: exact match">?</span></th>
             <th>Pattern</th>
             <th width="180">Route <span title="DIRECT: no proxy; TUN: route through TUN device; SOCKS5: use upstream SOCKS5 proxy">?</span></th>
             <th width="120">Actions</th>
           </tr>
         </thead>
         <tbody id="rules-body"></tbody>
       </table>
       <div id="rules-msg" style="margin-top:1rem;"></div>
       
       <div style="margin-top:1rem;">
         <label>
           <input type="checkbox" id="show-gfw"> Show GFW rules (<span id="gfw-count">0</span> rules)
         </label>
       </div>
       
       <div class="gfw-section" style="display:none;">
         <h3>GFW Rules</h3>
         <table>
           <thead>
             <tr>
               <th width="120">Match Type <span title="domain_suffix: match domain suffixes; host_glob: * and ? wildcards for hostnames; url_glob: * and ? wildcards for full URLs; exact: exact match">?</span></th>
               <th>Pattern</th>
             </tr>
           </thead>
           <tbody id="gfw-body"></tbody>
         </table>
       </div>
     </div>

    <!-- GFW Tab -->
    <div id="panel-gfw" class="panel">
      <div class="card">
        <h3>GFW List Configuration</h3>
        <div class="form-row">
          <label>Source URL</label>
          <input id="gfw-url" style="flex:3" placeholder="https://github.com/gfwlist/gfwlist/raw/refs/heads/master/gfwlist.txt">
          <button class="btn btn-secondary" onclick="saveGFWUrl()">Save URL</button>
        </div>
        <div class="form-row">
          <button class="btn btn-primary" onclick="fetchGFW()" id="gfw-fetch-btn">Fetch Now</button>
          <span id="gfw-msg"></span>
        </div>
      </div>
      <div class="card">
        <h3>Status</h3>
        <div id="gfw-status">Loading...</div>
      </div>
    </div>

    <!-- Test Tab -->
    <div id="panel-test" class="panel">
      <div class="card">
        <h3>Test URL Routing</h3>
        <div class="form-row">
          <input id="testurl" placeholder="https://example.com" style="flex:3">
          <button class="btn btn-primary" onclick="testUrl()">Test</button>
        </div>
        <pre id="testresult"></pre>
      </div>
     </div>

     <!-- Proxies Tab -->
     <div id="panel-proxies" class="panel">
       <div class="card">
         <h3>External Proxies</h3>
         <div class="form-row">
           <label>Default External Proxy</label>
           <select id="default-proxy-select">
             <option value="">(none)</option>
           </select>
           <button class="btn btn-primary" onclick="saveDefaultProxy()">Set Default</button>
         </div>
         <p class="hint">The special PAC return value <code>DEFAULT</code> will use this proxy.</p>
       </div>

       <div class="card">
         <h3>Upstream Proxies</h3>
         <table>
           <thead>
             <tr>
               <th>ID</th>
               <th>Address</th>
               <th>Auth</th>
               <th>Actions</th>
             </tr>
           </thead>
           <tbody id="proxies-body"></tbody>
         </table>
         <button class="btn btn-primary" style="margin-top:1rem;" onclick="showAddProxyModal()">Add Proxy</button>
       </div>
     </div>

     <!-- Add Proxy Modal -->
     <div id="add-proxy-modal" style="display:none; position:fixed; top:50%; left:50%; transform:translate(-50%,-50%); background:white; padding:2rem; border:1px solid #ccc; box-shadow:0 4px 16px rgba(0,0,0,0.2); z-index:1000; min-width:400px;">
       <h3>Add External Proxy</h3>
       <div class="form-row">
         <label>ID</label>
         <input id="new-proxy-id" placeholder="e.g., tor, myproxy">
       </div>
       <div class="form-row">
         <label>Address</label>
         <input id="new-proxy-addr" placeholder="127.0.0.1:1080">
       </div>
       <div class="form-row">
         <label>Username</label>
         <input id="new-proxy-user" placeholder="optional">
       </div>
       <div class="form-row">
         <label>Password</label>
         <input type="password" id="new-proxy-pass" placeholder="optional">
       </div>
       <div class="form-row">
         <button class="btn btn-secondary" onclick="testNewProxy()">Test</button>
         <span id="new-proxy-test-result" style="margin-left:1rem;"></span>
       </div>
       <div class="form-row" style="margin-top:1rem;">
         <button class="btn btn-primary" id="save-new-proxy-btn" onclick="saveNewProxy()" disabled>Save</button>
         <button class="btn" onclick="closeAddProxyModal()">Cancel</button>
       </div>
     </div>
     <div id="add-proxy-overlay" style="display:none; position:fixed; top:0; left:0; width:100%; height:100%; background:rgba(0,0,0,0.5); z-index:999;" onclick="closeAddProxyModal()"></div>

      <!-- Settings Tab -->
      <div id="panel-settings" class="panel">
        <div class="card">
          <h3>Proxy Listeners</h3>
          <div class="form-row">
            <label>HTTP Proxy (listen)</label>
            <input id="http-listen" value="0.0.0.0:8080">
          </div>
          <div class="form-row">
            <label>SOCKS5 Proxy (listen)</label>
            <input id="socks5-listen" value="0.0.0.0:1080">
          </div>
          <div class="form-row">
            <label>Admin UI (listen)</label>
            <input id="admin-listen" value="127.0.0.1:9090">
          </div>
          <div class="form-row">
            <label>HTTPS Proxy (listen)</label>
            <input id="https-listen" placeholder=":8443" value=":8443">
          </div>
        </div>

        <div class="card">
          <h3>TLS Settings</h3>
          <p class="hint">TLS certificate and key for HTTPS proxy. Restart required after changes.</p>
          <div class="form-row">
            <label>Certificate file</label>
            <input id="tls-certificate" placeholder="/path/to/cert.pem">
          </div>
          <div class="form-row">
            <label>Key file</label>
            <input id="tls-key" placeholder="/path/to/key.pem">
          </div>
          <div class="form-row">
            <label>CA file (optional)</label>
            <input id="tls-ca" placeholder="/path/to/ca.pem">
          </div>
          <div class="form-row">
            <label>Client auth</label>
            <select id="tls-client-auth">
              <option value="">None</option>
              <option value="request">Request</option>
              <option value="require_any">Require any</option>
              <option value="verify_if_given">Verify if given</option>
              <option value="require_verify">Require and verify</option>
            </select>
          </div>
        </div>

        <div class="card">
          <h3>Paths & Devices</h3>
          <div class="form-row">
            <label>PAC file path</label>
            <input id="pac-path" value="/etc/proxy.pac">
          </div>
          <div class="form-row">
            <label>TUN device</label>
            <input id="tun-device" value="tun0">
          </div>
        </div>

        <div class="card">
          <h3>Save Changes</h3>
          <p class="hint">Listener changes require a restart to take effect. Other changes are applied immediately.</p>
          <button class="btn btn-primary" onclick="saveConfig()">Save Configuration</button>
        </div>
      </div>

   </div>
    <script>
let allRules = [];
let allProxies = [];
let currentDefaultProxy = '';
let userRules = [];
let gfwRules = [];
     let editRowId = null;
     let gfwRowHeight = 0;
     let gfwVisibleStart = 0;
     let gfwVisibleEnd = 0;
     let gfwLoaded = false;

     const tooltips = {
       match: {
         domain_suffix: "Matches domain suffixes (e.g., .example.com)",
         host_glob: "Matches hostnames using * and ? wildcards",
         url_glob: "Matches full URLs using * and ? wildcards",
         exact: "Exact match"
       },
       route: {
         TUN: "Route through TUN device (VPN)",
         DIRECT: "Direct connection (no proxy)",
         SOCKS5: "Route through SOCKS5 upstream proxy"
       }
     };

     function switchTab(name) {
       document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
       document.querySelectorAll('.panel').forEach(p => p.classList.remove('active'));
       document.getElementById('panel-' + name).classList.add('active');
       const tabMap = {summary: 'Summary', connections: 'Connections', rules: 'Rules', gfw: 'GFW List', test: 'Test URL', proxies: 'Proxies', settings: 'Settings'};
       document.querySelectorAll('.tab').forEach(t => {
         if (t.textContent === tabMap[name]) t.classList.add('active');
       });
       if (name === 'summary') loadStatus();
       if (name === 'connections') loadConnections();
       if (name === 'rules') loadRules();
       if (name === 'gfw') { loadGFWStatus(); loadConfig(); }
  if (name === 'proxies') { loadProxies(); loadConfig(); }
  if (name === 'settings') { loadConfig(); }
     }

     function fmtDuration(ms) {
       var s = Math.floor(ms / 1000);
       var h = Math.floor(s / 3600);
       var m = Math.floor((s % 3600) / 60);
       s = s % 60;
       return String(h).padStart(2,'0') + ':' + String(m).padStart(2,'0') + ':' + String(s).padStart(2,'0');
     }

     async function loadStatus() {
       const res = await fetch('/api/status');
       const d = await res.json();
       document.getElementById('s-http').textContent = d.httpListen || '-';
       document.getElementById('s-socks5').textContent = d.socks5Listen || '-';
       document.getElementById('s-admin').textContent = d.adminListen || '-';
       var tunHtml = '';
       if (d.tunDevice) {
         tunHtml = d.tunDevice;
         if (d.tunIP) tunHtml += ' → ' + d.tunIP;
         tunHtml += ' <span class="status-dot ' + (d.tunUp ? 'up' : 'down') + '"></span>' + (d.tunUp ? 'up' : 'down');
       } else {
         tunHtml = '<span style="color:#999">not configured</span>';
       }
       document.getElementById('s-tun').innerHTML = tunHtml;
       document.getElementById('s-active').textContent = d.activeConns != null ? d.activeConns : '-';
       document.getElementById('s-total').textContent = d.totalConns != null ? d.totalConns : '-';
     }

     async function loadConnections() {
       const res = await fetch('/api/connections');
       const list = await res.json();
       const tbody = document.getElementById('conn-body');
       const empty = document.getElementById('conn-empty');
       const count = document.getElementById('conn-count');
       tbody.innerHTML = '';
       if (!list || list.length === 0) {
         empty.style.display = 'block';
         count.textContent = '';
         return;
       }
       empty.style.display = 'none';
       count.textContent = '(' + list.length + ')';
       var now = Date.now();
       list.forEach(function(c) {
         var tr = document.createElement('tr');
         var started = new Date(c.startedAt).getTime();
         var dur = fmtDuration(now - started);
         var routeCls = c.route === 'TUN' ? 'tun' : c.route === 'SOCKS5' ? 'socks5' : c.route === 'DIRECT' ? 'direct' : 'http';
         tr.innerHTML =
           '<td><code>' + escHtml(c.id) + '</code></td>' +
           '<td>' + escHtml(c.clientAddr) + '</td>' +
           '<td>' + escHtml(c.destAddr) + '</td>' +
           '<td><span class="badge badge-' + routeCls + '">' + escHtml(c.route) + '</span></td>' +
           '<td>' + escHtml(c.protocol) + '</td>' +
           '<td>' + dur + '</td>';
         tbody.appendChild(tr);
       });
     }

     async function loadRules() {
       try {
         const res = await fetch('/api/rules?t=' + Date.now());
         if (!res.ok) {
           const txt = await res.text();
           console.error('Failed to load rules:', res.status, txt);
           return;
         }
         allRules = await res.json();
         userRules = allRules.filter(r => r.source === 'user');
         gfwRules = allRules.filter(r => r.source === 'gfw');
         
         // Update User Rules heading
         const userHeading = document.querySelector('#panel-rules h3:nth-of-type(2)');
         if (userHeading) {
           userHeading.textContent = 'User Rules (' + userRules.length + ' user rules)';
         }
         
         renderRules();
         updateGFWCount();
       } catch (e) {
         console.error('Error loading rules:', e);
       }
     }

     function updateGFWCount() {
       const countEl = document.getElementById('gfw-count');
       if (countEl) {
         countEl.textContent = gfwRules.length;
       }
       
       // Update GFW Rules heading if section exists
       const gfwHeading = document.querySelector('.gfw-section h3');
       if (gfwHeading) {
         gfwHeading.textContent = 'GFW Rules (' + gfwRules.length + ' rules, read-only)';
       }
     }

     function renderRules() {
       const tbody = document.getElementById('rules-body');
       tbody.innerHTML = '';
       userRules.forEach((r, i) => {
         const tr = document.createElement('tr');
         tr.setAttribute('data-id', r.id);
         tr.setAttribute('data-route', r.route);
         tr.setAttribute('data-match', r.match);
         
         const isEditing = editRowId === r.id;
         const matchBadge = '<span class="badge badge-' + r.match + '" title="' + tooltips.match[r.match] + '">' + escHtml(r.match) + '</span>';
         const routeType = r.route.split(' ')[0];
         const routeBadge = '<span class="badge badge-' + routeType.toLowerCase() + '" title="' + tooltips.route[routeType] + '">' + escHtml(r.route) + '</span>';
         
         let patternCell;
         let actionsCell;
         
          if (isEditing) {
            const isValid = validatePattern(r.match, r.value);
            patternCell = '<input type="text" class="pattern-edit' + (isValid ? '' : ' invalid') + '" value="' + escHtml(r.value) + '" id="edit-value-' + r.id + '">';
            const matchSelect = '<select id="edit-match-' + r.id + '">' +
              ['domain_suffix','host_glob','url_glob','exact'].map(m => 
                '<option value="' + m + '"' + (m === r.match ? ' selected' : '') + '>' + m + '</option>'
              ).join('') + '</select>';
            let routeOptions = ['DIRECT','TUN','DEFAULT','SOCKS5 127.0.0.1:1080'];
            allProxies.forEach(p => routeOptions.push('EXTERNAL ' + p.id));
            const routeSelect = '<select id="edit-route-' + r.id + '">' +
              routeOptions.map(rt => 
                '<option value="' + rt + '"' + (rt === r.route ? ' selected' : '') + '>' + rt + '</option>'
              ).join('') + '</select>';
            actionsCell = '<span class="edit-controls">' +
              '<button class="save" onclick="saveEdit(\'' + r.id + '\')" title="Save">&#10003;</button>' +
              '<button class="cancel" onclick="cancelEdit()" title="Cancel">&#10007;</button>' +
              '</span>';
            
            tr.innerHTML =
              '<td><span class="drag-handle">&#9776;</span></td>' +
              '<td><span class="badge badge-user">user</span></td>' +
              '<td><span class="toggle on">&#10003;</span></td>' +
              '<td>' + matchSelect + '</td>' +
              '<td>' + patternCell + '</td>' +
              '<td>' + routeSelect + '</td>' +
              '<td>' + actionsCell + '</td>';
            
            // Attach input event for validation
            setTimeout(() => {
              const input = document.getElementById('edit-value-' + r.id);
              if (input) {
                input.focus();
                input.addEventListener('keyup', handleEditKeyup);
                input.addEventListener('change', handleEditChange);
              }
            }, 0);
          } else {
           patternCell = '<code class="pattern" onclick="startEdit(\'' + r.id + '\')">' + escHtml(r.value) + '</code>';
           actionsCell = '<button class="btn btn-sm btn-danger" onclick="deleteRule(\'' + r.id + '\')">Delete</button>';
           
           tr.innerHTML =
             '<td><span class="drag-handle">&#9776;</span></td>' +
             '<td><span class="badge badge-user">user</span></td>' +
             '<td><span class="toggle on" onclick="toggleRule(\'' + r.id + '\')">&#10003;</span></td>' +
             '<td title="' + tooltips.match[r.match] + '">' + matchBadge + '</td>' +
             '<td>' + patternCell + '</td>' +
             '<td title="' + tooltips.route[routeType] + '">' + routeBadge + '</td>' +
             '<td>' + actionsCell + '</td>';
         }
         tbody.appendChild(tr);
       });
     }

     function handleEditKeyup(e) {
       if (e.key === 'Enter') {
         e.preventDefault();
         if (editRowId) saveEdit(editRowId);
       } else if (e.key === 'Escape') {
         e.preventDefault();
         cancelEdit();
       }
     }

     function handleEditChange(e) {
       if (!editRowId) return;
       const rule = userRules.find(r => r.id === editRowId);
       if (!rule) return;
       const input = document.getElementById('edit-value-' + editRowId);
       const isValid = validatePattern(rule.match, input.value);
       const saveBtn = document.querySelector('.edit-controls .save');
       if (saveBtn) saveBtn.disabled = !isValid;
       if (input) input.classList.toggle('invalid', !isValid);
     }

     function validatePattern(match, value) {
       if (match === 'host_glob' || match === 'url_glob') {
         return value.trim() !== '';
       }
       return true;
     }

     function startEdit(id) {
       if (editRowId && editRowId !== id) {
         cancelEdit();
       }
       editRowId = id;
       renderRules();
     }

     async function saveEdit(id) {
       const rule = userRules.find(r => r.id === id);
       if (!rule) return;
       
       const match = document.getElementById('edit-match-' + id).value;
       const value = document.getElementById('edit-value-' + id).value.trim();
       const route = document.getElementById('edit-route-' + id).value;
       
       if (!validatePattern(match, value)) {
         alert('Pattern cannot be empty for host_glob or url_glob');
         return;
       }
       
       try {
         const res = await fetch('/api/rules/' + id, {
           method: 'PUT',
           headers: {'Content-Type':'application/json'},
           body: JSON.stringify({match, value, route})
         });
         if (res.ok) {
           editRowId = null;
           await loadRules();
         } else {
           alert('Failed to save rule');
         }
       } catch (e) {
         alert('Error: ' + e.message);
       }
     }

     function cancelEdit() {
       editRowId = null;
       renderRules();
      }

      function escHtml(s) { const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }

      async function addRule() {
       const rule = {
         match: document.getElementById('new-match').value,
         value: document.getElementById('new-value').value,
         route: document.getElementById('new-route').value
       };
       if (!rule.value) return;
       try {
         const res = await fetch('/api/rules', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(rule) });
         if (res.ok) {
           document.getElementById('new-value').value = '';
           await loadRules();
         } else {
           const err = await res.text();
           alert('Failed to add rule: ' + err);
         }
       } catch (e) {
          alert('Error adding rule: ' + e.message);
        }
      }

      async function toggleRule(id) {
       await fetch('/api/rules/' + id + '/toggle', { method: 'PUT' });
       loadRules();
     }

     async function deleteRule(id) {
       if (!confirm('Delete this rule?')) return;
       await fetch('/api/rules/' + id, { method: 'DELETE' });
       loadRules();
     }

      // GFW section
      async function loadGFWRules() {
        if (gfwLoaded) return;
        
        try {
          const res = await fetch('/api/rules');
          const all = await res.json();
          gfwRules = all.filter(r => r.source === 'gfw');
          gfwLoaded = true;
          updateGFWCount();
          
          const container = document.querySelector('.gfw-section');
          const tbody = document.getElementById('gfw-body');
          if (container && tbody && gfwRules.length > 0) {
            // Render one row to measure height
            const tempTr = document.createElement('tr');
            tempTr.className = 'gfw-row';
            tempTr.innerHTML = '<td><span class="badge badge-host_glob">host_glob</span></td><td><code>example</code></td>';
            tbody.appendChild(tempTr);
            gfwRowHeight = tempTr.offsetHeight;
            tbody.innerHTML = '';
            
            // Fallback if measurement failed
            if (gfwRowHeight <= 0) {
              gfwRowHeight = 40;
            }
            
            // Set tbody height for scrollability
            tbody.style.height = (gfwRules.length * gfwRowHeight) + 'px';
            
            // Attach scroll listener
            container.addEventListener('scroll', handleGFWScroll);
            
            // Initial render
            renderGFWSlice();
          }
        } catch (e) {
          console.error('Failed to load GFW rules:', e);
        }
      }

     function handleGFWScroll() {
       renderGFWSlice();
     }

     function renderGFWSlice() {
       const container = document.querySelector('.gfw-section');
       const tbody = document.getElementById('gfw-body');
       if (!container || !tbody || gfwRules.length === 0) return;
       
       const scrollTop = container.scrollTop;
       const clientHeight = container.clientHeight;
       
       const buffer = 5;
       const start = Math.max(0, Math.floor(scrollTop / gfwRowHeight) - buffer);
       const end = Math.min(gfwRules.length, start + Math.ceil(clientHeight / gfwRowHeight) + 2 * buffer);
       
       gfwVisibleStart = start;
       gfwVisibleEnd = end;
       
       const slice = gfwRules.slice(start, end);
       tbody.innerHTML = '';
       
       slice.forEach((r, i) => {
         const tr = document.createElement('tr');
         tr.className = 'gfw-row';
         const rowTop = (start + i) * gfwRowHeight;
         tr.style.position = 'absolute';
         tr.style.top = rowTop + 'px';
         tr.style.width = '100%';
         
         const matchBadge = '<span class="badge badge-' + r.match + '" title="' + tooltips.match[r.match] + '">' + escHtml(r.match) + '</span>';
         
         tr.innerHTML =
           '<td title="' + tooltips.match[r.match] + '">' + matchBadge + '</td>' +
           '<td><code>' + escHtml(r.value) + '</code></td>';
         tbody.appendChild(tr);
       });
     }

     function initGFWCheckbox() {
       const checkbox = document.getElementById('show-gfw');
       if (checkbox) {
         // Restore state from localStorage
         const saved = localStorage.getItem('showGFW');
         if (saved === 'true') {
           checkbox.checked = true;
           document.querySelector('.gfw-section').style.display = 'block';
           // Ensure GFW table loads and renders
           loadGFWRules();
         }
         
         checkbox.addEventListener('change', (e) => {
           const section = document.querySelector('.gfw-section');
           section.style.display = e.target.checked ? 'block' : 'none';
           localStorage.setItem('showGFW', e.target.checked);
           if (e.target.checked) {
             loadGFWRules();
           }
         });
       }
     }

     async function loadGFWStatus() {
       const res = await fetch('/api/gfw/status');
       const data = await res.json();
       const el = document.getElementById('gfw-status');
       if (!data.configured) {
         el.innerHTML = '<span class="status-line">GFW fetcher not configured.</span>';
         return;
       }
       el.innerHTML =
         '<div class="status-line">Configured: <strong>Yes</strong></div>' +
         '<div class="status-line">Last fetch: ' + (data.lastFetch ? new Date(data.lastFetch).toLocaleString() : 'Never') + '</div>' +
         '<div class="status-line">Rules: <strong>' + (data.ruleCount || 0) + '</strong></div>' +
         (data.lastError ? '<div class="status-line error">Error: ' + escHtml(data.lastError) + '</div>' : '') +
         (data.etag ? '<div class="status-line">ETag: ' + escHtml(data.etag) + '</div>' : '');
     }

     async function saveGFWUrl() {
       const url = document.getElementById('gfw-url').value;
       const res = await fetch('/api/gfw/config', { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify({sourceURL: url}) });
       const msg = document.getElementById('gfw-msg');
       msg.innerHTML = res.ok ? '<span class="success">Saved</span>' : '<span class="error">Failed</span>';
     }

     async function fetchGFW() {
       const btn = document.getElementById('gfw-fetch-btn');
       const msg = document.getElementById('gfw-msg');
       btn.disabled = true;
       btn.textContent = 'Fetching...';
       msg.innerHTML = '';
       try {
         const res = await fetch('/api/gfw/fetch', { method: 'POST' });
         const data = await res.json();
         if (data.error) {
           msg.innerHTML = '<span class="error">' + escHtml(data.error) + '</span>';
         } else if (data.updated) {
           msg.innerHTML = '<span class="success">Updated: ' + data.ruleCount + ' rules</span>';
         } else {
           msg.innerHTML = '<span class="success">No changes (' + data.ruleCount + ' rules)</span>';
         }
       } catch (e) {
         msg.innerHTML = '<span class="error">' + e.message + '</span>';
       }
       btn.disabled = false;
       btn.textContent = 'Fetch Now';
       loadGFWStatus();
     }

     async function testUrl() {
       const url = document.getElementById('testurl').value;
       const res = await fetch('/test?url=' + encodeURIComponent(url));
       const data = await res.json();
       document.getElementById('testresult').textContent = JSON.stringify(data, null, 2);
     }

      // Proxy management functions
      function loadProxies() {
        fetch('/api/proxies').then(r => r.json()).then(data => {
          allProxies = data;
          renderProxies();
          updateRouteOptions();
        }).catch(err => {
          console.error('Failed to load proxies:', err);
        });
      }

      function renderProxies() {
        const tbody = document.getElementById('proxies-body');
        tbody.innerHTML = '';
        allProxies.forEach(p => {
          const tr = document.createElement('tr');
          tr.innerHTML = '<td>' + p.id + '</td><td>' + p.address + '</td><td>' + (p.has_auth ? 'Yes' : 'No') + '</td><td><button class="btn btn-sm btn-secondary" onclick="editProxy(\'' + p.id + '\')">Edit</button> <button class="btn btn-sm btn-danger" onclick="deleteProxy(\'' + p.id + '\')">Delete</button> <button class="btn btn-sm btn-secondary" onclick="testProxy(\'' + p.id + '\')">Test</button></td>';
          tbody.appendChild(tr);
        });
      }

      function showAddProxyModal() {
        document.getElementById('add-proxy-modal').style.display = 'block';
        document.getElementById('add-proxy-overlay').style.display = 'block';
        document.getElementById('new-proxy-id').value = '';
        document.getElementById('new-proxy-addr').value = '';
        document.getElementById('new-proxy-user').value = '';
        document.getElementById('new-proxy-pass').value = '';
        document.getElementById('new-proxy-test-result').textContent = '';
        document.getElementById('save-new-proxy-btn').disabled = true;
      }

      function closeAddProxyModal() {
        document.getElementById('add-proxy-modal').style.display = 'none';
        document.getElementById('add-proxy-overlay').style.display = 'none';
      }

      function testNewProxy() {
        const addr = document.getElementById('new-proxy-addr').value.trim();
        const user = document.getElementById('new-proxy-user').value.trim();
        const pass = document.getElementById('new-proxy-pass').value;
        const resultSpan = document.getElementById('new-proxy-test-result');
        if (!addr) {
          resultSpan.textContent = 'Address required';
          resultSpan.style.color = '#c00';
          return;
        }
        resultSpan.textContent = 'Testing...';
        resultSpan.style.color = '';
        fetch('/api/proxies/test', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({address: addr, username: user, password: pass})
        })
        .then(r => r.json())
        .then(data => {
          if (data.success) {
            resultSpan.textContent = 'Success!';
            resultSpan.style.color = '#090';
            document.getElementById('save-new-proxy-btn').disabled = false;
          } else {
            resultSpan.textContent = 'Failed: ' + data.error;
            resultSpan.style.color = '#c00';
            document.getElementById('save-new-proxy-btn').disabled = true;
          }
        })
        .catch(e => {
          resultSpan.textContent = 'Error: ' + e.message;
          resultSpan.style.color = '#c00';
          document.getElementById('save-new-proxy-btn').disabled = true;
        });
      }

      function saveNewProxy() {
        const id = document.getElementById('new-proxy-id').value.trim();
        const addr = document.getElementById('new-proxy-addr').value.trim();
        const user = document.getElementById('new-proxy-user').value.trim();
        const pass = document.getElementById('new-proxy-pass').value;
        if (!id || !addr) {
          alert('ID and Address are required');
          return;
        }
        fetch('/api/proxies', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({id: id, address: addr, username: user, password: pass})
        })
        .then(r => {
          if (r.ok) {
            closeAddProxyModal();
            loadProxies();
          } else {
            return r.text().then(txt => { throw new Error(txt); });
          }
          return r;
        })
        .catch(e => alert('Failed to add: ' + e.message));
      }

      function editProxy(id) {
        const p = allProxies.find(x => x.id === id);
        if (!p) return;
        const newId = prompt('Edit ID (cannot change):', p.id);
        if (newId === null) return;
        const newAddr = prompt('Address:', p.address);
        if (newAddr === null) return;
        const newUser = prompt('Username (leave unchanged if empty):', p.username || '');
        const newPass = prompt('Password (leave unchanged if empty):', '');
        const payload = { address: newAddr };
        if (newUser !== '') payload.username = newUser;
        if (newPass !== '') payload.password = newPass;
        fetch('/api/proxies/' + encodeURIComponent(id), {
          method: 'PUT',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify(payload)
        })
        .then(r => {
          if (r.ok) return r.json();
          return r.text().then(txt => { throw new Error(txt); });
        })
        .then(data => {
          loadProxies();
          alert('Updated');
        })
        .catch(e => alert('Error: ' + e.message));
      }

      function deleteProxy(id) {
        if (!confirm('Delete proxy ' + id + '?')) return;
        fetch('/api/proxies/' + encodeURIComponent(id), { method: 'DELETE' })
          .then(r => {
            if (r.ok) {
              loadProxies();
            } else {
              r.text().then(txt => alert('Failed: ' + txt));
            }
          })
          .catch(e => alert('Error: ' + e.message));
      }

      function testProxy(id) {
        fetch('/api/proxies/' + encodeURIComponent(id) + '/test', { method: 'POST' })
          .then(r => r.json())
          .then(data => {
            if (data.success) {
              alert('Proxy test succeeded');
            } else {
              alert('Proxy test failed: ' + data.error);
            }
          })
          .catch(e => alert('Error: ' + e.message));
      }

        function loadConfig() {
          // Load main proxy config
          fetch('/api/config').then(r => r.json()).then(cfg => {
            currentDefaultProxy = cfg.default_external_proxy || '';
            document.getElementById('default-proxy-select').value = currentDefaultProxy;
            if (document.getElementById('http-listen')) document.getElementById('http-listen').value = cfg.http_listen || '';
            if (document.getElementById('socks5-listen')) document.getElementById('socks5-listen').value = cfg.socks5_listen || '';
            if (document.getElementById('admin-listen')) document.getElementById('admin-listen').value = cfg.admin_listen || '';
            if (document.getElementById('https-listen')) document.getElementById('https-listen').value = cfg.https_listen || '';
            if (document.getElementById('pac-path')) document.getElementById('pac-path').value = cfg.pac_path || '';
            if (document.getElementById('tun-device')) document.getElementById('tun-device').value = cfg.tun_device || '';
            // TLS fields
            if (document.getElementById('tls-certificate')) document.getElementById('tls-certificate').value = cfg.tls?.certificate_file || '';
            if (document.getElementById('tls-key')) document.getElementById('tls-key').value = cfg.tls?.key_file || '';
            if (document.getElementById('tls-ca')) document.getElementById('tls-ca').value = cfg.tls?.ca_file || '';
            if (document.getElementById('tls-client-auth')) document.getElementById('tls-client-auth').value = cfg.tls?.client_auth || '';
          }).catch(err => {
            console.error('Failed to load config:', err);
          });

          // Load GFW config to show source URL
          fetch('/api/gfw/config')
            .then(r => r.json())
            .then(data => {
              if (document.getElementById('gfw-url')) {
                document.getElementById('gfw-url').value = data.sourceURL || '';
              }
            })
            .catch(err => {
              console.error('Failed to load GFW config:', err);
            });
        }

       function saveConfig() {
         const payload = {
           default_external_proxy: document.getElementById('default-proxy-select').value,
           http_listen: document.getElementById('http-listen').value,
           socks5_listen: document.getElementById('socks5-listen').value,
           admin_listen: document.getElementById('admin-listen').value,
           https_listen: document.getElementById('https-listen').value,
           pac_path: document.getElementById('pac-path').value,
           tun_device: document.getElementById('tun-device').value,
           tls: {
             certificate_file: document.getElementById('tls-certificate').value,
             key_file: document.getElementById('tls-key').value,
             ca_file: document.getElementById('tls-ca').value,
             client_auth: document.getElementById('tls-client-auth').value
           }
         };
         fetch('/api/config', {
           method: 'PUT',
           headers: {'Content-Type': 'application/json'},
           body: JSON.stringify(payload)
         })
         .then(r => r.json())
         .then(data => {
           alert('Config saved' + (data.restart_required ? ' (restart required for listeners/TLS)' : ''));
         })
         .catch(e => alert('Error: ' + e.message));
       }
       
       function updateRouteOptions() {
        const base = ['DIRECT','TUN','DEFAULT','SOCKS5 127.0.0.1:1080'];
        allProxies.forEach(p => base.push('EXTERNAL ' + p.id));
        const addSelect = document.getElementById('new-route');
        if (addSelect) {
          addSelect.innerHTML = base.map(rt => '<option value="' + rt + '">' + rt + '</option>').join('');
        }
        // For edit selects, we could also update, but they are dynamic; simplest is to reload page or use edit prompt
      }

      // Override loadProxies to also populate default proxy select
      loadProxies = function() {
        fetch('/api/proxies').then(r => r.json()).then(data => {
          allProxies = data;
          renderProxies();
          updateRouteOptions();
          const sel = document.getElementById('default-proxy-select');
          if (sel) {
            sel.innerHTML = '<option value="">(none)</option>';
            data.forEach(p => {
              const opt = document.createElement('option');
              opt.value = p.id;
              opt.textContent = p.id;
              sel.appendChild(opt);
            });
            if (currentDefaultProxy) sel.value = currentDefaultProxy;
          }
        }).catch(err => console.error('Failed to load proxies:', err));
      };

      function saveDefaultProxy() {
        const val = document.getElementById('default-proxy-select').value;
        fetch('/api/config', {
          method: 'PUT',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({default_external_proxy: val})
        })
        .then(r => r.json())
        .then(data => {
          currentDefaultProxy = val;
          alert('Default proxy saved');
        })
        .catch(e => alert('Error: ' + e.message));
      }

       function saveConfig() {
         const payload = {
           default_external_proxy: document.getElementById('default-proxy-select').value,
           http_listen: document.getElementById('http-listen').value,
           socks5_listen: document.getElementById('socks5-listen').value,
           admin_listen: document.getElementById('admin-listen').value,
           https_listen: document.getElementById('https-listen').value,
           pac_path: document.getElementById('pac-path').value,
           tun_device: document.getElementById('tun-device').value,
           tls: {
             certificate_file: document.getElementById('tls-certificate').value,
             key_file: document.getElementById('tls-key').value,
             ca_file: document.getElementById('tls-ca').value,
             client_auth: document.getElementById('tls-client-auth').value
           }
         };
         fetch('/api/config', {
           method: 'PUT',
           headers: {'Content-Type': 'application/json'},
           body: JSON.stringify(payload)
         })
         .then(r => r.json())
         .then(data => {
           alert('Config saved' + (data.restart_required ? ' (restart required for listeners/TLS)' : ''));
         })
         .catch(e => alert('Error: ' + e.message));
       }
      
      setInterval(loadStatus, 5000);
     setInterval(function() {
       if (document.getElementById('panel-connections').classList.contains('active')) {
         loadConnections();
       }
     }, 2000);
     
     // Initialize GFW checkbox on page load
     document.addEventListener('DOMContentLoaded', initGFWCheckbox);
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"configured": true,
		"sourceURL":  h.fetcher.SourceURL,
		"localPath":  h.fetcher.LocalPath,
		"userPath":   h.userPath,
	})
}

// handlePutGFWConfig updates GFW configuration and persists to config file.
func (h *Handler) handlePutGFWConfig(w http.ResponseWriter, r *http.Request) {
	if h.fetcher == nil {
		http.Error(w, "GFW fetcher not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		SourceURL string `json:"sourceURL"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.SourceURL == "" {
		http.Error(w, "sourceURL is required", http.StatusBadRequest)
		return
	}

	// Update in-memory fetcher
	h.fetcher.SourceURL = req.SourceURL

	// Persist to config file if we know the path
	if h.configPath != "" {
		// Read existing config
		data, err := os.ReadFile(h.configPath)
		if err != nil {
			log.Printf("[Admin] Failed to read config for update: %v", err)
		} else {
			var cfg FullConfig
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				log.Printf("[Admin] Failed to parse config for update: %v", err)
			} else {
				cfg.GFW.SourceURL = req.SourceURL
				out, err := yaml.Marshal(&cfg)
				if err != nil {
					log.Printf("[Admin] Failed to marshal config: %v", err)
				} else {
					// Write atomically
					tmpPath := h.configPath + ".tmp"
					if err := os.WriteFile(tmpPath, out, 0o644); err != nil {
						log.Printf("[Admin] Failed to write config tmp: %v", err)
					} else if err := os.Rename(tmpPath, h.configPath); err != nil {
						os.Remove(tmpPath)
						log.Printf("[Admin] Failed to rename config: %v", err)
					} else {
						log.Printf("[Admin] Updated GFW source_url in config: %s", req.SourceURL)
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// regeneratePAC merges user and GFW rules and writes the PAC file.
func (h *Handler) regeneratePAC() {
	if h.userPath == "" || h.gfwPath == "" || h.pacPath == "" {
		return
	}
	if err := gfw.RegeneratePAC(h.userPath, h.gfwPath, h.pacPath, h.defaultExternalProxy); err != nil {
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
			"has_auth": (p.Username != "" || p.Password != ""),
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(proxies)
}

// handleCreateProxy adds a new external proxy.
func (h *Handler) handleCreateProxy(w http.ResponseWriter, r *http.Request) {
	var newProxy pac.ExternalProxy
	if err := json.NewDecoder(r.Body).Decode(&newProxy); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if newProxy.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if newProxy.Address == "" {
		http.Error(w, "address is required", http.StatusBadRequest)
		return
	}
	// Validate address format and loop prevention
	status := h.statusProvider.Status()
	if err := validateProxyAddress(newProxy.Address, status.HTTPListen, status.SOCKS5Listen, status.AdminListen); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Reserved ID check
	switch newProxy.ID {
	case "DIRECT", "TUN", "DEFAULT":
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
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":       newProxy.ID,
		"address":  newProxy.Address,
		"has_auth": (newProxy.Username != "" || newProxy.Password != ""),
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
	data, err := os.ReadFile(h.configPath)
	if err != nil {
		http.Error(w, "Cannot read config", http.StatusInternalServerError)
		return
	}
	var cfg FullConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		http.Error(w, "Cannot parse config", http.StatusInternalServerError)
		return
	}
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
	timeout := 5 * time.Second
	conn, err := socks5.DialWithAuth("tcp", proxy.Address, &socks5.Auth{Username: proxy.Username, Password: proxy.Password}, timeout)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	conn.Close()
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// handleTestProxy tests an arbitrary unsaved proxy configuration.
func (h *Handler) handleTestProxy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address  string `json:"address"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Address == "" {
		http.Error(w, "address required", http.StatusBadRequest)
		return
	}
	timeout := 5 * time.Second
	conn, err := socks5.DialWithAuth("tcp", req.Address, &socks5.Auth{Username: req.Username, Password: req.Password}, timeout)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	conn.Close()
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
		Address  string `json:"address"`
		Username string `json:"username"`
		Password string `json:"password"`
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

// handleGetConfig returns current proxy configuration (listeners, pac_path, tun_device, default_external_proxy).
func (h *Handler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	status := h.statusProvider.Status()
	cfg := struct {
		HTTPListen           string `json:"http_listen"`
		SOCKS5Listen         string `json:"socks5_listen"`
		AdminListen          string `json:"admin_listen"`
		HTTPSListen          string `json:"https_listen"`
		PACPath              string `json:"pac_path"`
		TUNDevice            string `json:"tun_device"`
		DefaultExternalProxy string `json:"default_external_proxy"`
		TLS                  struct {
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
	// Read config file to get PACPath, TUNDevice, DefaultExternalProxy, HTTPSListen, TLS
	data, err := os.ReadFile(h.configPath)
	if err == nil {
		var full FullConfig
		if err := yaml.Unmarshal(data, &full); err == nil {
			cfg.PACPath = full.Proxy.PACPath
			cfg.TUNDevice = full.Proxy.TUNDevice
			cfg.DefaultExternalProxy = full.Proxy.DefaultExternalProxy
			cfg.HTTPSListen = full.Proxy.HTTPSListen
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
		TLS                  struct {
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
	if upd.TUNDevice != "" {
		cfg.Proxy.TUNDevice = upd.TUNDevice
	}
	if upd.DefaultExternalProxy != "" && upd.DefaultExternalProxy != cfg.Proxy.DefaultExternalProxy {
		cfg.Proxy.DefaultExternalProxy = upd.DefaultExternalProxy
		h.router.SetDefaultProxyID(upd.DefaultExternalProxy)
		h.defaultExternalProxy = upd.DefaultExternalProxy
		h.regeneratePAC()
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
	// Write updated config
	if err := h.writeFullConfig(cfg); err != nil {
		log.Printf("[Admin] Failed to write config: %v", err)
		http.Error(w, "Failed to save config", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":           "ok",
		"restart_required": restartRequired,
	})
}
