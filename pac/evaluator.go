package pac

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
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
	engineType  string       // "native"
	lastModTime atomic.Int64 // last modified time in Unix nanoseconds
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
	e.engineType = "native"
	e.lastModTime.Store(modTimeNano)
	if e.cache != nil {
		e.cache.InvalidateAll()
	}
	log.Println("PAC reloaded with native engine successfully")
	return nil
}

// Evaluate executes the PAC evaluation for the given host, using cache if enabled.
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
	r := engine.Evaluate(host)

	// Cache the result
	if e.cache != nil {
		e.cache.Set(host, r)
	}
	return r
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
