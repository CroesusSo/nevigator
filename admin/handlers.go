package admin

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dop251/goja"
	"gopkg.in/yaml.v3"

	"github.com/yourorg/proxy-pac/gfw"
	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/status"
)

// FullConfig mirrors the main package's Config for reading/writing the YAML.
type FullConfig struct {
	Proxy struct {
		HTTPListen   string `yaml:"http_listen"`
		SOCKS5Listen string `yaml:"socks5_listen"`
		AdminListen  string `yaml:"admin_listen"`
		PACPath      string `yaml:"pac_path"`
		TUNDevice    string `yaml:"tun_device"`
	} `yaml:"proxy"`
	PAC struct {
		LocalIP        string        `yaml:"local_ip"`
		DNSTimeout     time.Duration `yaml:"dns_timeout"`
		EvalTimeout    time.Duration `yaml:"eval_timeout"`
		EnableDNSCache bool          `yaml:"enable_dns_cache"`
	} `yaml:"pac"`
	GFW struct {
		SourceURL      string        `yaml:"source_url"`
		LocalRulesPath string        `yaml:"local_rules_path"`
		UserRulesPath  string        `yaml:"user_rules_path"`
		UseTUN         bool          `yaml:"use_tun"`
		UpdateInterval time.Duration `yaml:"update_interval"`
		FetchOnStartup bool          `yaml:"fetch_on_startup"`
		HTTPTimeout    time.Duration `yaml:"http_timeout"`
	} `yaml:"gfw"`
}

// Handler provides admin UI and API endpoints.
type Handler struct {
	evaluator      *pac.Evaluator
	pacPath        string
	fetcher        *gfw.Fetcher
	userPath       string
	gfwPath        string
	statusProvider status.StatusProvider
	configPath     string // path to config YAML
}

// NewHandler creates a new admin handler.
func NewHandler(evaluator *pac.Evaluator, pacPath string, fetcher *gfw.Fetcher, userPath, gfwPath, configPath string, statusProvider status.StatusProvider) *Handler {
	return &Handler{
		evaluator:      evaluator,
		pacPath:        pacPath,
		fetcher:        fetcher,
		userPath:       userPath,
		gfwPath:        gfwPath,
		statusProvider: statusProvider,
		configPath:     configPath,
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

  </div>
   <script>
     let allRules = [];
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
       const tabMap = {summary: 'Summary', connections: 'Connections', rules: 'Rules', gfw: 'GFW List', test: 'Test URL'};
       document.querySelectorAll('.tab').forEach(t => {
         if (t.textContent === tabMap[name]) t.classList.add('active');
       });
       if (name === 'summary') loadStatus();
       if (name === 'connections') loadConnections();
       if (name === 'rules') loadRules();
       if (name === 'gfw') loadGFWStatus();
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
            const routeSelect = '<select id="edit-route-' + r.id + '">' +
              ['DIRECT','TUN','SOCKS5 127.0.0.1:1080'].map(rt => 
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

     loadStatus();
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
	if h.statusProvider == nil {
		json.NewEncoder(w).Encode(status.ServerStatus{})
		return
	}
	json.NewEncoder(w).Encode(h.statusProvider.Status())
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
					if err := os.WriteFile(tmpPath, out, 0644); err != nil {
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
	if err := gfw.RegeneratePAC(h.userPath, h.gfwPath, h.pacPath); err != nil {
		log.Printf("[Admin] PAC regeneration failed: %v", err)
	}
}
