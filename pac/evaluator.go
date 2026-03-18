package pac

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	"github.com/fsnotify/fsnotify"
)

// defaultPACContent is the minimal PAC script used when no PAC file exists.
const defaultPACContent = `function FindProxyForURL(url, host) {
  if (dnsDomainIs(host, "google.com")) {
    return "TUN";
  }
  return "DIRECT";
}`

type compiledPAC struct {
	prog    *goja.Program
	modTime time.Time
}

// Evaluator implements the PAC evaluation logic.
type Evaluator struct {
	cfg         EvaluatorConfig
	current     atomic.Value // *compiledPAC or *NativeRuleEngine
	stopped     chan struct{}
	closeOnce   sync.Once
	dnsCache    *dnsCache
	cache       *EvaluatorCache
	engineType  string       // "js" or "native"
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
	e.dnsCache = newDNSCache(5 * time.Minute)
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

	// Try native engine if PAC contains __PAC_RULES__
	if strings.Contains(string(src), "__PAC_RULES__") {
		engine, err := NewNativeRuleEngine(string(src), e.cfg.FallbackRoute)
		if err != nil {
			log.Printf("[WARN] Native engine build failed: %v; falling back to JS", err)
		} else {
			e.current.Store(engine)
			e.engineType = "native"
			e.lastModTime.Store(modTimeNano)
			if e.cache != nil {
				e.cache.InvalidateAll()
			}
			log.Println("PAC reloaded with native engine successfully")
			return nil
		}
	}

	// Fallback to JS compilation
	vm := goja.New()
	registerBuiltins(vm, e.cfg, e.dnsCache)
	prog, err := goja.Compile(e.cfg.PACPath, string(src), false)
	if err != nil {
		return fmt.Errorf("compile pac: %v", err)
	}
	// Execute to validate
	if _, err := vm.RunProgram(prog); err != nil {
		return fmt.Errorf("execute pac for validation: %v", err)
	}
	// Atomically swap
	newPAC := &compiledPAC{prog: prog, modTime: modTime}
	e.current.Store(newPAC)
	e.engineType = "js"
	e.lastModTime.Store(modTimeNano)
	// Invalidate cache on reload
	if e.cache != nil {
		e.cache.InvalidateAll()
	}
	log.Println("PAC reloaded with JS engine successfully")
	return nil
}

// evaluateJS executes the PAC script using goja for the given host.
func (e *Evaluator) evaluateJS(pac *compiledPAC, host string) Route {
	vm := goja.New()
	registerBuiltins(vm, e.cfg, e.dnsCache)
	// Run PAC to define FindProxyForURL
	if _, err := vm.RunProgram(pac.prog); err != nil {
		log.Printf("PAC runtime error: %v", err)
		return e.cfg.FallbackRoute
	}
	fnVal := vm.Get("FindProxyForURL")
	if fnVal == nil {
		log.Printf("PAC missing FindProxyForURL")
		return e.cfg.FallbackRoute
	}
	fn, ok := goja.AssertFunction(fnVal)
	if !ok {
		log.Printf("PAC FindProxyForURL is not a function")
		return e.cfg.FallbackRoute
	}
	// Evaluate with timeout
	type result struct{ route Route }
	ch := make(chan result, 1)
	go func() {
		ret, err := fn(goja.Undefined(), vm.ToValue(""), vm.ToValue(host))
		if err != nil {
			log.Printf("PAC evaluation error: %v", err)
			ch <- result{e.cfg.FallbackRoute}
			return
		}
		ch <- result{parsePACResult(ret.String())}
	}()
	select {
	case r := <-ch:
		return r.route
	case <-time.After(e.cfg.EvalTimeout):
		vm.Interrupt(fmt.Errorf("evaluation timeout after %v", e.cfg.EvalTimeout))
		<-ch // wait for goroutine to finish
		log.Printf("PAC timeout for host %s", host)
		return e.cfg.FallbackRoute
	}
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

	var r Route
	switch v := val.(type) {
	case *NativeRuleEngine:
		r = v.Evaluate(host)
	case *compiledPAC:
		r = e.evaluateJS(v, host)
	default:
		log.Printf("[ERROR] Unknown engine type in Evaluator.current")
		return e.cfg.FallbackRoute
	}

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
	e.closeOnce.Do(func() { close(e.stopped) })
	if e.cache != nil {
		e.cache.Close()
	}
	return nil
}
