package pac

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
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
	cfg       EvaluatorConfig
	current   atomic.Value // *compiledPAC
	stopped   chan struct{}
	closeOnce sync.Once
	dnsCache  *dnsCache
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
	// Check if up-to-date
	if old := e.current.Load(); old != nil {
		if old.(*compiledPAC).modTime.Equal(modTime) {
			return nil
		}
	}
	src, err := os.ReadFile(e.cfg.PACPath)
	if err != nil {
		return fmt.Errorf("read pac file: %v", err)
	}
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
	log.Println("PAC reloaded successfully")
	return nil
}

// Evaluate executes the PAC script for the given host and returns a Route.
// If evaluation fails or times out, returns the configured fallback route.
func (e *Evaluator) Evaluate(host string) Route {
	pacVal := e.current.Load()
	if pacVal == nil {
		return e.cfg.FallbackRoute
	}
	pac := pacVal.(*compiledPAC)
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

// Close stops background processes. Safe to call multiple times.
func (e *Evaluator) Close() error {
	e.closeOnce.Do(func() { close(e.stopped) })
	return nil
}
