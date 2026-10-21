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
let currentEditProxyId = null;
let currentEditProxyKind = null;

  const tooltips = {
    match: {
      domain_suffix: "Matches domain suffixes (e.g., .example.com)",
      host_glob: "Matches hostnames using * and ? wildcards",
      url_glob: "Matches full URLs using * and ? wildcards",
      exact: "Exact match"
    },
    route: {
      DIRECT: "Direct connection (no proxy)",
      SOCKS5: "Route through SOCKS5 upstream proxy"
    }
   };

  // formatDuration converts time.Duration string (from API) to a user-friendly format
  function formatDuration(d) {
    if (!d) return '';
    // d is like "500ms", "5m", "200ms", "24h"
    return String(d);
  }

  function toggleSSHFields() {
    const kindSelect = document.getElementById('new-proxy-kind');
    if (!kindSelect) return;
    const kind = kindSelect.value;
    const sshFields = document.getElementsByClassName('ssh-fields');
    const isSSH = kind === 'ssh';
    for (let i = 0; i < sshFields.length; i++) {
      sshFields[i].style.display = isSSH ? '' : 'none';
    }
    // Also show/hide SSH port field specifically (it's always shown for SSH)
    const sshPortField = document.getElementById('new-proxy-ssh-port');
    if (sshPortField) {
      sshPortField.parentElement.parentElement.style.display = isSSH ? '' : 'none';
    }
  }

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
   if (name === 'rules') {loadProxies();  loadRules(); }
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

  function toggleSSHAuthFields() {
    const authMethod = document.getElementById('new-ssh-auth-method').value;
    const passwordField = document.getElementById('ssh-password-field');
    const keyPathField = document.getElementById('ssh-key-path-field');
    if (authMethod === 'password') {
      passwordField.style.display = '';
      keyPathField.style.display = 'none';
    } else if (authMethod === 'key') {
      passwordField.style.display = 'none';
      keyPathField.style.display = '';
    }
  }

  async function loadStatus() {
    const res = await fetch('/api/status');
    const d = await res.json();
    document.getElementById('s-http').textContent = d.httpListen || '-';
    document.getElementById('s-socks5').textContent = d.socks5Listen || '-';
    document.getElementById('s-https').textContent = d.httpsListen || '-';
    document.getElementById('s-admin').textContent = d.adminListen || '-';
    document.getElementById('s-active').textContent = d.activeConns != null ? d.activeConns : '-';
    document.getElementById('s-total').textContent = d.totalConns != null ? d.totalConns : '-';
    // Load tunnels
    renderTunnels(d.sshTunnels);
  }

  function renderTunnels(tunnels) {
    const tbody = document.getElementById('tunnels-body');
    const empty = document.getElementById('tunnels-empty');
    tbody.innerHTML = '';
    const entries = Object.entries(tunnels || {});
    if (entries.length === 0) {
      empty.style.display = 'block';
      return;
    }
    empty.style.display = 'none';
    entries.forEach(([id, info]) => {
      const tr = document.createElement('tr');
      tr.innerHTML = `<td>${escHtml(id)}</td><td>${escHtml(info.remoteAddr || '-')}</td><td>${escHtml(info.state || '-')}</td><td>${info.lastError ? escHtml(info.lastError) : '-'}</td>`;
      tbody.appendChild(tr);
    });
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
      var routeCls = c.route === 'SOCKS5' ? 'socks5' : c.route === 'DIRECT' ? 'direct' : 'http';
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
        let routeOptions = ['DIRECT','DEFAULT','SOCKS5 127.0.0.1:1080'];
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
    // Disallow url_glob if disabled
    if (rule.match === 'url_glob' && window.configEnableURLGlob === false) {
      alert('URL-glob rules are disabled in Settings. Enable them to add such rules.');
      return;
    }
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
        tempTr.innerHTML = '<td width="120"><span class="badge badge-host_glob">host_glob</span></td><td width="60"><code>example</code></td><td><code>example</code></td>';
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
       '<td width="120" title="' + tooltips.match[r.match] + '">' + matchBadge + '</td>' +
       '<td width="60"><code>' + escHtml(r.action) + '</code></td>' +
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
      '<div class="status-line">Enabled: <strong>' + (data.enabled ? 'Yes' : 'No') + '</strong></div>' +
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

  function saveGFWEnabled() {
    const enabled = document.getElementById('gfw-enabled').checked;
    fetch('/api/gfw/config', {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({ enabled: enabled })
    })
    .then(r => r.json())
    .then(() => {
      // Update disabled state of URL input and fetch button
      const disabled = !enabled;
      document.getElementById('gfw-url').disabled = disabled;
      document.getElementById('gfw-fetch-btn').disabled = disabled;
      loadGFWStatus();
    })
    .catch(err => {
      console.error('Failed to save GFW enabled:', err);
    });
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
       let keepaliveHtml = '';
       if (p.keepalive_enabled) {
         keepaliveHtml = '<span style="color:green">Enabled</span>';
         if (p.keepalive_interval) {
           keepaliveHtml += ' ' + p.keepalive_interval;
         }
       } else {
         keepaliveHtml = '<span style="color:#999">Disabled</span>';
       }
       tr.innerHTML = '<td>' + p.id + '</td><td>' + p.address + '</td><td>' + (p.has_auth ? 'Yes' : 'No') + '</td><td>' + keepaliveHtml + '</td><td><button class="btn btn-sm btn-secondary" onclick="editProxy(\'' + p.id + '\')">Edit</button> <button class="btn btn-sm btn-danger" onclick="deleteProxy(\'' + p.id + '\')">Delete</button> <button class="btn btn-sm btn-secondary" onclick="testProxy(\'' + p.id + '\')">Test</button></td>';
       tbody.appendChild(tr);
     });
   }

    function showAddSocks5Modal() {
      document.getElementById('add-socks5-modal').style.display = 'block';
      document.getElementById('add-socks5-overlay').style.display = 'block';
      document.getElementById('new-socks5-id').value = '';
      document.getElementById('new-socks5-addr').value = '';
      document.getElementById('new-socks5-user').value = '';
      document.getElementById('new-socks5-pass').value = '';
      document.getElementById('new-socks5-test-result').textContent = '';
      document.getElementById('save-new-socks5-btn').disabled = true;
    }

    function closeAddSocks5Modal() {
      document.getElementById('add-socks5-modal').style.display = 'none';
      document.getElementById('add-socks5-overlay').style.display = 'none';
    }

     function showAddSSHModal() {
       document.getElementById('add-ssh-modal').style.display = 'block';
       document.getElementById('add-ssh-overlay').style.display = 'block';
       document.getElementById('new-ssh-id').value = '';
       document.getElementById('new-ssh-addr').value = '';
       document.getElementById('new-ssh-port').value = '22';
       document.getElementById('new-ssh-user').value = '';
       document.getElementById('new-ssh-auth-method').value = 'password';
       document.getElementById('new-ssh-pass').value = '';
       document.getElementById('new-ssh-private-key-path').value = '';
       document.getElementById('new-ssh-host-key-check').value = 'strict';
       document.getElementById('new-ssh-keepalive-enabled').checked = false;
       document.getElementById('new-ssh-keepalive-interval').value = '';
       document.getElementById('new-ssh-test-result').textContent = '';
       document.getElementById('save-new-ssh-btn').disabled = true;
       // Initialize auth field visibility
       toggleSSHAuthFields();
     }

    function closeAddSocks5Modal() {
      document.getElementById('add-socks5-modal').style.display = 'none';
      document.getElementById('add-socks5-overlay').style.display = 'none';
    }

    function closeAddSSHModal() {
      document.getElementById('add-ssh-modal').style.display = 'none';
      document.getElementById('add-ssh-overlay').style.display = 'none';
    }

   function testNewSocks5Proxy() {
     const addr = document.getElementById('new-socks5-addr').value.trim();
     const user = document.getElementById('new-socks5-user').value.trim();
     const pass = document.getElementById('new-socks5-pass').value;
     const resultSpan = document.getElementById('new-socks5-test-result');
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
         document.getElementById('save-new-socks5-btn').disabled = false;
       } else {
         resultSpan.textContent = 'Failed: ' + data.error;
         resultSpan.style.color = '#c00';
         document.getElementById('save-new-socks5-btn').disabled = true;
       }
     })
     .catch(e => {
       resultSpan.textContent = 'Error: ' + e.message;
       resultSpan.style.color = '#c00';
       document.getElementById('save-new-socks5-btn').disabled = true;
     });
   }

   function testNewSSHProxy() {
     const addr = document.getElementById('new-ssh-addr').value.trim();
     const user = document.getElementById('new-ssh-user').value.trim();
     const port = document.getElementById('new-ssh-port').value;
     const authMethod = document.getElementById('new-ssh-auth-method').value;
     const pass = document.getElementById('new-ssh-pass').value;
     const privateKeyPath = document.getElementById('new-ssh-private-key-path').value.trim();
     const resultSpan = document.getElementById('new-ssh-test-result');
     if (!addr) {
       resultSpan.textContent = 'Address required';
       resultSpan.style.color = '#c00';
       return;
     }
     resultSpan.textContent = 'Testing...';
     resultSpan.style.color = '';
    // Build the payload for the test endpoint
    const data = {
      address: addr,
      username: user,
      password: pass,
      kind: 'ssh',
      auth_method: authMethod,
      private_key_path: privateKeyPath,
      ssh_port: parseInt(port) || 22,
      host_key_check: document.getElementById('new-ssh-host-key-check').value
    };
     fetch('/api/proxies/test', {
       method: 'POST',
       headers: {'Content-Type': 'application/json'},
       body: JSON.stringify(data)
     })
     .then(r => r.json())
     .then(data => {
       if (data.success) {
         resultSpan.textContent = 'Success!';
         resultSpan.style.color = '#090';
         document.getElementById('save-new-ssh-btn').disabled = false;
       } else {
         resultSpan.textContent = 'Failed: ' + data.error;
         resultSpan.style.color = '#c00';
         document.getElementById('save-new-ssh-btn').disabled = true;
       }
     })
     .catch(e => {
       resultSpan.textContent = 'Error: ' + e.message;
       resultSpan.style.color = '#c00';
       document.getElementById('save-new-ssh-btn').disabled = true;
     });
   }

    function saveNewSocks5Proxy() {
      const id = document.getElementById('new-socks5-id').value.trim();
      const addr = document.getElementById('new-socks5-addr').value.trim();
      const user = document.getElementById('new-socks5-user').value.trim();
      const pass = document.getElementById('new-socks5-pass').value;
      if (!id || !addr) {
        alert('ID and Address are required');
        return;
      }
      const data = {id: id, address: addr, username: user, password: pass, kind: 'socks5'};
      fetch('/api/proxies', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(data)
      })
      .then(r => {
        if (r.ok) {
          closeAddSocks5Modal();
          loadProxies();
        } else {
          return r.text().then(txt => { throw new Error(txt); });
        }
      })
      .catch(e => alert('Error: ' + e.message));
    }

     function saveNewSSHProxy() {
       const id = document.getElementById('new-ssh-id').value.trim();
       const addr = document.getElementById('new-ssh-addr').value.trim();
       const user = document.getElementById('new-ssh-user').value.trim();
       const pass = document.getElementById('new-ssh-pass').value;
       const port = document.getElementById('new-ssh-port').value;
       const authMethod = document.getElementById('new-ssh-auth-method').value;
       const privateKeyPath = document.getElementById('new-ssh-private-key-path').value.trim();
       if (!id || !addr) {
         alert('ID and Address are required');
         return;
       }
       const data = {id: id, address: addr, username: user, password: pass, kind: 'ssh'};
       data.auth_method = authMethod;
       if (authMethod === 'key') {
         data.private_key_path = privateKeyPath;
       } else {
         data.password = pass;
       }
       data.ssh_port = parseInt(port) || 22;
       // Keepalive
       const keepaliveEnabled = document.getElementById('new-ssh-keepalive-enabled').checked;
       const keepaliveInterval = document.getElementById('new-ssh-keepalive-interval').value.trim();
       if (keepaliveEnabled) {
         data.keepalive = { enabled: true };
         if (keepaliveInterval) {
           data.keepalive.interval = keepaliveInterval;
         }
       }
       fetch('/api/proxies', {
         method: 'POST',
         headers: {'Content-Type': 'application/json'},
         body: JSON.stringify(data)
       })
       .then(r => {
         if (r.ok) {
           closeAddSSHModal();
           loadProxies();
         } else {
           return r.text().then(txt => { throw new Error(txt); });
         }
       })
       .catch(e => alert('Error: ' + e.message));
     }

   function editProxy(id) {
     const p = allProxies.find(x => x.id === id);
     if (!p) return;
     // Close any open edit modal
     closeEditSocks5Modal();
     closeEditSSHModal();
     // Determine kind (default to socks5 if unspecified)
     const kind = p.kind || 'socks5';
     if (kind === 'ssh') {
       showEditSSHModal(id);
     } else {
       showEditSocks5Modal(id);
     }
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

  function toggleEditSSHAuthFields() {
    const authMethod = document.getElementById('edit-ssh-auth-method').value;
    document.getElementById('edit-ssh-password-field').style.display = authMethod === 'password' ? '' : 'none';
    document.getElementById('edit-ssh-key-path-field').style.display = authMethod === 'key' ? '' : 'none';
  }

  function toggleEditSSHKeepalive() {
    const enabled = document.getElementById('edit-ssh-keepalive-enabled').checked;
    const intervalSpan = document.querySelector('#edit-ssh-keepalive-interval').parentElement;
    if (intervalSpan) intervalSpan.style.display = enabled ? '' : 'none';
  }

  function showEditSocks5Modal(id) {
    fetch('/api/proxies/' + encodeURIComponent(id))
      .then(r => r.json())
      .then(proxy => {
        document.getElementById('edit-socks5-id').textContent = proxy.id;
        document.getElementById('edit-socks5-addr').value = proxy.address || '';
        document.getElementById('edit-socks5-user').value = proxy.username || '';
        document.getElementById('edit-socks5-pass').value = proxy.password || '';
        document.getElementById('edit-socks5-test-result').textContent = '';
        document.getElementById('edit-socks5-modal').style.display = 'block';
        document.getElementById('edit-socks5-overlay').style.display = 'block';
        currentEditProxyId = id;
        currentEditProxyKind = 'socks5';
      })
      .catch(err => {
        console.error('Failed to fetch proxy:', err);
        alert('Error loading proxy: ' + err.message);
      });
  }

  function showEditSSHModal(id) {
    fetch('/api/proxies/' + encodeURIComponent(id))
      .then(r => r.json())
      .then(proxy => {
        document.getElementById('edit-ssh-id').textContent = proxy.id;
        document.getElementById('edit-ssh-addr').value = proxy.address || '';
        document.getElementById('edit-ssh-port').value = proxy.ssh_port || 22;
        document.getElementById('edit-ssh-user').value = proxy.username || '';
        document.getElementById('edit-ssh-auth-method').value = proxy.auth_method || 'password';
        document.getElementById('edit-ssh-pass').value = proxy.password || '';
        document.getElementById('edit-ssh-private-key-path').value = proxy.private_key_path || '';
        document.getElementById('edit-ssh-host-key-check').value = proxy.host_key_check || 'strict';
        document.getElementById('edit-ssh-host-key-fingerprint').value = proxy.host_key_fingerprint || '';
        // Keepalive
        const keepaliveEnabled = proxy.keepalive && proxy.keepalive.enabled;
        const keepaliveInterval = proxy.keepalive && proxy.keepalive.interval ? String(proxy.keepalive.interval) : '';
        document.getElementById('edit-ssh-keepalive-enabled').checked = keepaliveEnabled;
        document.getElementById('edit-ssh-keepalive-interval').value = keepaliveInterval;
        // Toggle field visibility based on current values
        toggleEditSSHAuthFields();
        toggleEditSSHKeepalive();
        document.getElementById('edit-ssh-test-result').textContent = '';
        document.getElementById('edit-ssh-modal').style.display = 'block';
        document.getElementById('edit-ssh-overlay').style.display = 'block';
        currentEditProxyId = id;
        currentEditProxyKind = 'ssh';
      })
      .catch(err => {
        console.error('Failed to fetch proxy:', err);
        alert('Error loading proxy: ' + err.message);
      });
  }

  function saveEditSocks5Proxy() {
    const id = currentEditProxyId;
    if (!id) return;
    const addr = document.getElementById('edit-socks5-addr').value.trim();
    const user = document.getElementById('edit-socks5-user').value.trim();
    const pass = document.getElementById('edit-socks5-pass').value;
    const payload = { address: addr };
    if (user) payload.username = user;
    if (pass) payload.password = pass;
    fetch('/api/proxies/' + encodeURIComponent(id), {
      method: 'PUT',
      headers: {'Content-Type':'application/json'},
      body: JSON.stringify(payload)
    })
    .then(r => {
      if (r.ok) {
        closeEditSocks5Modal();
        loadProxies();
        alert('Updated');
      } else {
        return r.text().then(txt => { throw new Error(txt); });
      }
    })
    .catch(e => alert('Error: ' + e.message));
  }

  function saveEditSSHProxy() {
    const id = currentEditProxyId;
    if (!id) return;
    const addr = document.getElementById('edit-ssh-addr').value.trim();
    const user = document.getElementById('edit-ssh-user').value.trim();
    const pass = document.getElementById('edit-ssh-pass').value;
    const port = document.getElementById('edit-ssh-port').value;
    const authMethod = document.getElementById('edit-ssh-auth-method').value;
    const privateKeyPath = document.getElementById('edit-ssh-private-key-path').value.trim();
    const hostKeyCheck = document.getElementById('edit-ssh-host-key-check').value;
    const hostKeyFingerprint = document.getElementById('edit-ssh-host-key-fingerprint').value.trim();
    const keepaliveEnabled = document.getElementById('edit-ssh-keepalive-enabled').checked;
    const keepaliveInterval = document.getElementById('edit-ssh-keepalive-interval').value.trim();

    const payload = { address: addr };
    if (user) payload.username = user;
    if (pass) payload.password = pass;
    payload.kind = 'ssh';
    payload.ssh_port = parseInt(port) || 22;
    payload.auth_method = authMethod;
    if (authMethod === 'key') {
      payload.private_key_path = privateKeyPath;
    } else {
      payload.password = pass;
    }
    payload.host_key_check = hostKeyCheck;
    if (hostKeyFingerprint) payload.host_key_fingerprint = hostKeyFingerprint;
    if (keepaliveEnabled) {
      payload.keepalive_enabled = true;
      if (keepaliveInterval) payload.keepalive_interval = keepaliveInterval;
    } else {
      payload.keepalive_enabled = false;
    }

    fetch('/api/proxies/' + encodeURIComponent(id), {
      method: 'PUT',
      headers: {'Content-Type':'application/json'},
      body: JSON.stringify(payload)
    })
    .then(r => {
      if (r.ok) {
        closeEditSSHModal();
        loadProxies();
        alert('Updated');
      } else {
        return r.text().then(txt => { throw new Error(txt); });
      }
    })
    .catch(e => alert('Error: ' + e.message));
  }

  function closeEditSocks5Modal() {
    document.getElementById('edit-socks5-modal').style.display = 'none';
    document.getElementById('edit-socks5-overlay').style.display = 'none';
    currentEditProxyId = null;
    currentEditProxyKind = null;
  }

  function closeEditSSHModal() {
    document.getElementById('edit-ssh-modal').style.display = 'none';
    document.getElementById('edit-ssh-overlay').style.display = 'none';
    currentEditProxyId = null;
    currentEditProxyKind = null;
  }

  function testProxyFromEdit() {
    if (!currentEditProxyId) return;
    fetch('/api/proxies/' + encodeURIComponent(currentEditProxyId) + '/test', { method: 'POST' })
      .then(r => r.json())
      .then(data => {
        const socks5Modal = document.getElementById('edit-socks5-modal');
        const sshModal = document.getElementById('edit-ssh-modal');
        let resultSpan;
        if (socks5Modal.style.display === 'block') {
          resultSpan = document.getElementById('edit-socks5-test-result');
        } else if (sshModal.style.display === 'block') {
          resultSpan = document.getElementById('edit-ssh-test-result');
        }
        if (resultSpan) {
          if (data.success) {
            resultSpan.textContent = 'Success!';
            resultSpan.style.color = '#090';
          } else {
            resultSpan.textContent = 'Failed: ' + data.error;
            resultSpan.style.color = '#c00';
          }
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
        // TLS fields
        if (document.getElementById('tls-certificate')) document.getElementById('tls-certificate').value = cfg.tls?.certificate_file || '';
        if (document.getElementById('tls-key')) document.getElementById('tls-key').value = cfg.tls?.key_file || '';
        if (document.getElementById('tls-ca')) document.getElementById('tls-ca').value = cfg.tls?.ca_file || '';
        if (document.getElementById('tls-client-auth')) document.getElementById('tls-client-auth').value = cfg.tls?.client_auth || '';

        // SOCKS5 inbound auth + handshake timeout.
        // The password is never returned by the API, so the field stays blank;
        // leaving it blank on save keeps the stored password.
        const sAuth = cfg.socks5_auth || {};
        if (document.getElementById('socks5-auth-enabled')) {
            document.getElementById('socks5-auth-enabled').checked = !!(sAuth.username || sAuth.has_password);
        }
        if (document.getElementById('socks5-auth-username')) document.getElementById('socks5-auth-username').value = sAuth.username || '';
        if (document.getElementById('socks5-auth-password')) document.getElementById('socks5-auth-password').value = '';
        if (document.getElementById('socks5-auth-password')) {
            document.getElementById('socks5-auth-password').placeholder = sAuth.has_password ? 'unchanged (set)' : 'unchanged if blank';
        }
        if (document.getElementById('socks5-handshake-timeout')) document.getElementById('socks5-handshake-timeout').value = cfg.socks5_handshake_timeout || '';

        // TCP keepalive. Only prefill the timings when probes are on, so a
        // disabled config shows blanks rather than the defaults in use.
        const ka = cfg.tcp_keepalive || {};
        if (document.getElementById('tcp-keepalive-enabled')) document.getElementById('tcp-keepalive-enabled').checked = !!ka.enabled;
        if (document.getElementById('tcp-keepalive-idle')) document.getElementById('tcp-keepalive-idle').value = ka.enabled ? (ka.idle || '') : '';
        if (document.getElementById('tcp-keepalive-interval')) document.getElementById('tcp-keepalive-interval').value = ka.enabled ? (ka.interval || '') : '';
        if (document.getElementById('tcp-keepalive-count')) document.getElementById('tcp-keepalive-count').value = ka.enabled ? (ka.count ?? '') : '';

         // PAC settings
         if (document.getElementById('local-ip')) document.getElementById('local-ip').value = cfg.pac?.local_ip || '';
         if (document.getElementById('dns-timeout')) document.getElementById('dns-timeout').value = formatDuration(cfg.pac?.dns_timeout);
         if (document.getElementById('eval-timeout')) document.getElementById('eval-timeout').value = formatDuration(cfg.pac?.eval_timeout);
         if (document.getElementById('enable-dns-cache')) document.getElementById('enable-dns-cache').checked = cfg.pac?.enable_dns_cache || false;
         if (document.getElementById('dns-max-entries')) document.getElementById('dns-max-entries').value = cfg.pac?.dns_max_entries || 100;
        if (document.getElementById('dns-ttl')) document.getElementById('dns-ttl').value = formatDuration(cfg.pac?.dns_ttl);
        if (document.getElementById('cache-enabled')) document.getElementById('cache-enabled').checked = cfg.pac?.cache_enabled || false;
        if (document.getElementById('cache-max-size')) document.getElementById('cache-max-size').value = cfg.pac?.cache_max_size || 10000;
        if (document.getElementById('cache-ttl')) document.getElementById('cache-ttl').value = formatDuration(cfg.pac?.cache_ttl);
        
         // URL-glob
         if (document.getElementById('enable-url-glob')) {
             document.getElementById('enable-url-glob').checked = cfg.pac?.enable_url_glob || false;
         }
         window.configEnableURLGlob = !!cfg.pac?.enable_url_glob;
      }).catch(err => {
        console.error('Failed to load config:', err);
      });

      // Load GFW config to show source URL and enabled state
      fetch('/api/gfw/config')
        .then(r => r.json())
        .then(data => {
          if (document.getElementById('gfw-url')) {
            document.getElementById('gfw-url').value = data.sourceURL || '';
          }
          const cb = document.getElementById('gfw-enabled');
          if (cb) {
            cb.checked = data.enabled;
            // Disable URL and fetch button if disabled
            const disabled = !data.enabled;
            document.getElementById('gfw-url').disabled = disabled;
            document.getElementById('gfw-fetch-btn').disabled = disabled;
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
        tls: {
          certificate_file: document.getElementById('tls-certificate').value,
          key_file: document.getElementById('tls-key').value,
          ca_file: document.getElementById('tls-ca').value,
          client_auth: document.getElementById('tls-client-auth').value
        },
        pac: {
          local_ip: document.getElementById('local-ip').value,
          dns_timeout: document.getElementById('dns-timeout').value,
          eval_timeout: document.getElementById('eval-timeout').value,
          enable_dns_cache: document.getElementById('enable-dns-cache').checked,
          dns_max_entries: parseInt(document.getElementById('dns-max-entries').value, 10),
          dns_ttl: document.getElementById('dns-ttl').value,
          cache_enabled: document.getElementById('cache-enabled').checked,
          cache_max_size: parseInt(document.getElementById('cache-max-size').value, 10),
          cache_ttl: document.getElementById('cache-ttl').value,
          enable_url_glob: document.getElementById('enable-url-glob').checked
        }
      };

      // SOCKS5 inbound auth. An empty username and password clears the
      // requirement; a blank password with a username keeps the stored one.
      const sAuthEnabled = document.getElementById('socks5-auth-enabled').checked;
      const sAuthUser = document.getElementById('socks5-auth-username').value.trim();
      const sAuthPass = document.getElementById('socks5-auth-password').value;
      if (!sAuthEnabled) {
        payload.socks5_auth = { username: '', password: '' };
      } else if (sAuthUser || sAuthPass) {
        payload.socks5_auth = { username: sAuthUser, password: sAuthPass };
      }

      const hsTimeout = document.getElementById('socks5-handshake-timeout').value.trim();
      if (hsTimeout) payload.socks5_handshake_timeout = hsTimeout;

      const kaEnabled = document.getElementById('tcp-keepalive-enabled').checked;
      const kaIdle = document.getElementById('tcp-keepalive-idle').value.trim();
      const kaInterval = document.getElementById('tcp-keepalive-interval').value.trim();
      const kaCount = document.getElementById('tcp-keepalive-count').value.trim();
      payload.tcp_keepalive = { enabled: kaEnabled };
      if (kaEnabled) {
        if (kaIdle) payload.tcp_keepalive.idle = kaIdle;
        if (kaInterval) payload.tcp_keepalive.interval = kaInterval;
        if (kaCount !== '') payload.tcp_keepalive.count = parseInt(kaCount, 10);
      }

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
     const base = ['DIRECT','DEFAULT','SOCKS5 127.0.0.1:1080'];
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

   setInterval(loadStatus, 5000);
   setInterval(function() {
     if (document.getElementById('panel-connections').classList.contains('active')) {
       loadConnections();
     }
   }, 2000);
  setInterval(function() {
    if (document.getElementById('panel-connections').classList.contains('active')) {
      loadConnections();
    }
  }, 2000);
  
  // Initialize UI state on load
  initGFWCheckbox();
  loadConfig();
  
  // Show/hide URL-glob warning based on settings
  document.getElementById('new-match').addEventListener('change', function() {
    var warning = document.getElementById('url-glob-warning');
    if (warning) {
      warning.style.display = (this.value === 'url_glob' && window.configEnableURLGlob === false) ? 'block' : 'none';
    }
  });