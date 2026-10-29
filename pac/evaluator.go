package pac

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// defaultPACContent is the minimal PAC script used when no PAC file exists.
// It MUST contain __PAC_RULES__ structured rules; JS evaluation is removed.
const defaultPACContent = `var __PAC_RULES__ = {"version":1,"rules":[]};
function FindProxyForURL(url, host) { return "DIRECT"; }`

type Evaluator struct {
	cfg         EvaluatorConfig
	current     atomic.Value // *NativeRuleEngine
	stopped     chan struct{}
	closeOnce   sync.Once
	dnsCache    *dnsCache
	cache       *EvaluatorCache
	lastModTime atomic.Int64 // last modified time in Unix nanoseconds

	// hostFallbacksMu guards hostFallbacks, which the assembly layer updates
	// at runtime when the GFW list source changes.
	hostFallbacksMu sync.RWMutex
	hostFallbacks   map[string]Route
}

// NewEvaluator creates a new Evaluator with the given config.
// It loads and compiles the PAC file at startup.
func NewEvaluator(cfg EvaluatorConfig) (*Evaluator, error) {
	e := &Evaluator{
		cfg:     cfg,
		stopped: make(chan struct{}),
	}
	// Always create dnsCache; caching behavior controlled by enabled flag
	// Set DNS cache defaults if not specified
	if cfg.DNSTTL == 0 {
		cfg.DNSTTL = 5 * time.Minute
	}
	if cfg.DNSMaxEntries == 0 {
		cfg.DNSMaxEntries = 100
	}
	e.dnsCache = newDNSCache(cfg.DNSTTL, cfg.DNSMaxEntries)
	e.dnsCache.enabled = cfg.EnableDNSCache
	// If custom DNS servers configured, set custom resolver
	if len(cfg.CustomDNSServers) > 0 {
		e.dnsCache.resolver = CreateCustomResolver(cfg.CustomDNSServers)
		log.Printf("[PAC] Using custom DNS: %v", cfg.CustomDNSServers)
	}
	// Initialize cache if enabled
	// Set defaults if not specified
	if cfg.CacheEnabled {
		if cfg.CacheMaxSize == 0 {
			cfg.CacheMaxSize = 10000
		}
		if cfg.CacheTTL == 0 {
			cfg.CacheTTL = 10 * time.Minute
		}
	}
	e.cache = newEvaluatorCache(cacheConfig{
		enabled:         cfg.CacheEnabled,
		maxSize:         cfg.CacheMaxSize,
		ttl:             cfg.CacheTTL,
		cleanupInterval: time.Minute,
	})
	// Seed the per-host fallback routes from the configuration.
	e.hostFallbacks = make(map[string]Route, len(cfg.HostFallbacks))
	for host, route := range cfg.HostFallbacks {
		e.hostFallbacks[normalizeHostKey(host)] = route
	}
	// Start periodic cache stats logging if cache enabled
	if e.cache != nil {
		go e.logCacheStatsLoop()
	}
	// Initial load
	if err := e.reload(); err != nil {
		return nil, err
	}
	// Start file watcher for hot reload
	go e.watch()
	return e, nil
}

// watch monitors the PAC file directory for changes using fsnotify.
func (e *Evaluator) watch() {
	dir := filepath.Dir(e.cfg.PACPath)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Failed to create fsnotify watcher: %v", err)
		return
	}
	defer watcher.Close()
	if err := watcher.Add(dir); err != nil {
		log.Printf("Failed to watch PAC directory %s: %v", dir, err)
		return
	}
	log.Printf("Watching PAC file %s for changes", e.cfg.PACPath)
	for {
		select {
		case <-e.stopped:
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Name == e.cfg.PACPath && (event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create) {
				log.Println("PAC file changed, reloading...")
				e.Reload()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

// Reload recompiles the PAC file and atomically swaps it in.
func (e *Evaluator) Reload() {
	if err := e.reload(); err != nil {
		log.Printf("PAC reload failed: %v", err)
	}
}

func (e *Evaluator) reload() error {
	info, err := os.Stat(e.cfg.PACPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat pac file: %v", err)
		}
		// PAC file doesn't exist — create default
		if err := os.WriteFile(e.cfg.PACPath, []byte(defaultPACContent), 0644); err != nil {
			return fmt.Errorf("create default pac file: %v", err)
		}
		log.Printf("Created default PAC file at %s", e.cfg.PACPath)
		// Re-stat to get modTime
		info, err = os.Stat(e.cfg.PACPath)
		if err != nil {
			return fmt.Errorf("stat newly created pac file: %v", err)
		}
	}
	modTime := info.ModTime()
	modTimeNano := modTime.UnixNano()
	// Check if up-to-date
	if e.lastModTime.Load() == modTimeNano {
		return nil
	}

	src, err := os.ReadFile(e.cfg.PACPath)
	if err != nil {
		return fmt.Errorf("read pac file: %v", err)
	}

	// Native engine only: PAC must contain __PAC_RULES__
	engine, err := NewNativeRuleEngine(string(src), e.cfg.FallbackRoute, e.cfg.EnableDeepestMatch)
	if err != nil {
		log.Printf("[WARN] Native engine build failed: %v", err)
		// No fallback to JS; keep current engine if possible
		// Return error but don't replace engine; current stays as is
		return fmt.Errorf("native engine build: %w", err)
	}
	e.current.Store(engine)
	e.lastModTime.Store(modTimeNano)
	if e.cache != nil {
		e.cache.InvalidateAll()
	}
	log.Println("PAC reloaded with native engine successfully")
	return nil
}

// Evaluate executes the PAC evaluation for the given host, using cache if enabled.
//
// A rule in the PAC file always wins. When no rule matches, the per-host
// fallback route is consulted before the global fallback route, so a host
// that must follow a specific path can be pinned even while no rule for it
// exists.
func (e *Evaluator) Evaluate(host string) Route {
	// Try cache first
	if e.cache != nil {
		if r, ok := e.cache.Get(host); ok {
			return r
		}
	}

	val := e.current.Load()
	if val == nil {
		return e.cfg.FallbackRoute
	}

	// Native engine only
	engine, ok := val.(*NativeRuleEngine)
	if !ok {
		log.Printf("[ERROR] Expected NativeRuleEngine but got %T", val)
		return e.cfg.FallbackRoute
	}
	r, matched := engine.EvaluateWithMatch(host)
	if !matched {
		if fallback, ok := e.hostFallback(host); ok {
			r = fallback
		}
	}

	// Cache the result
	if e.cache != nil {
		e.cache.Set(host, r)
	}
	return r
}

// hostFallback returns the per-host fallback route for host, if one is set.
func (e *Evaluator) hostFallback(host string) (Route, bool) {
	e.hostFallbacksMu.RLock()
	defer e.hostFallbacksMu.RUnlock()
	route, ok := e.hostFallbacks[normalizeHostKey(host)]
	return route, ok
}

// SetHostFallback pins the route used for a host when no PAC rule matches
// it. It replaces any previous fallback for that host.
//
// The cached evaluation results are dropped, because a host that previously
// fell back to the global route now takes the pinned one.
func (e *Evaluator) SetHostFallback(host string, route Route) {
	key := normalizeHostKey(host)
	e.hostFallbacksMu.Lock()
	e.hostFallbacks[key] = route
	e.hostFallbacksMu.Unlock()
	if e.cache != nil {
		e.cache.InvalidateAll()
	}
}

// RemoveHostFallback removes the per-host fallback route for a host.
func (e *Evaluator) RemoveHostFallback(host string) {
	key := normalizeHostKey(host)
	e.hostFallbacksMu.Lock()
	delete(e.hostFallbacks, key)
	e.hostFallbacksMu.Unlock()
	if e.cache != nil {
		e.cache.InvalidateAll()
	}
}

// normalizeHostKey reduces a host name to the form used to look up rules and
// per-host fallbacks: lower case, without a port.
func normalizeHostKey(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.TrimSuffix(host, ".")
}

// CacheStats returns current cache statistics (hits, misses, size, maxSize, hitRate).
// Returns zeros if cache is disabled.
func (e *Evaluator) CacheStats() (hits, misses, size, maxSize int, hitRate float64) {
	if e.cache == nil {
		return 0, 0, 0, 0, 0
	}
	return e.cache.Stats()
}

// logCacheStatsLoop periodically logs cache statistics.
func (e *Evaluator) logCacheStatsLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			hits, misses, size, maxSize, hitRate := e.cache.Stats()
			log.Printf("[PAC cache] hits=%d misses=%d size=%d/%d hit_rate=%.2f%%",
				hits, misses, size, maxSize, hitRate*100)
		case <-e.stopped:
			return
		}
	}
}

// Close stops background processes. Safe to call multiple times.
func (e *Evaluator) Close() error {
	e.closeOnce.Do(func() {
		close(e.stopped)
		if e.cache != nil {
			e.cache.Close()
		}
	})
	return nil
}
