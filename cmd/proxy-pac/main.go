package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/yourorg/proxy-pac/admin"
	"github.com/yourorg/proxy-pac/gfw"
	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/proxy"
	"github.com/yourorg/proxy-pac/router"
	"github.com/yourorg/proxy-pac/ssh"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Proxy struct {
		HTTPListen           string              `yaml:"http_listen"`
		SOCKS5Listen         string              `yaml:"socks5_listen"`
		AdminListen          string              `yaml:"admin_listen"`
		HTTPSListen          string              `yaml:"https_listen"`
		PACPath              string              `yaml:"pac_path"`
		TUNDevice            string              `yaml:"tun_device"`
		DefaultExternalProxy string              `yaml:"default_external_proxy"`
		ExternalProxies      []pac.ExternalProxy `yaml:"external_proxies"`
		TLS                  pac.TLSConfig       `yaml:"tls"`
	} `yaml:"proxy"`
	PAC struct {
		LocalIP        string        `yaml:"local_ip"`
		DNSTimeout     time.Duration `yaml:"dns_timeout"`
		EvalTimeout    time.Duration `yaml:"eval_timeout"`
		EnableDNSCache bool          `yaml:"enable_dns_cache"`
		EnableURLGlob  bool          `yaml:"enable_url_glob"`
		DNSMaxEntries  int           `yaml:"dns_max_entries"`
		DNSTTL         time.Duration `yaml:"dns_ttl"`
		CacheEnabled   bool          `yaml:"cache_enabled"`
		CacheMaxSize   int           `yaml:"cache_max_size"`
		CacheTTL       time.Duration `yaml:"cache_ttl"`
	} `yaml:"pac"`
	GFW struct {
		Disabled         bool          `yaml:"disabled" json:"disabled"`
		SourceURL        string        `yaml:"source_url"`
		LocalRulesPath   string        `yaml:"local_rules_path"`
		UserRulesPath    string        `yaml:"user_rules_path"`
		UseTUN           bool          `yaml:"use_tun"`
		UpdateInterval   time.Duration `yaml:"update_interval"`
		FetchOnStartup   bool          `yaml:"fetch_on_startup"`
		HTTPTimeout      time.Duration `yaml:"http_timeout"`
		CustomDNSServers []string      `yaml:"custom_dns_servers"` // Optional custom DNS for GFW fetcher
	} `yaml:"gfw"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	// Set defaults
	if cfg.Proxy.HTTPListen == "" {
		cfg.Proxy.HTTPListen = ":8080"
	}
	if cfg.Proxy.SOCKS5Listen == "" {
		cfg.Proxy.SOCKS5Listen = ":1080"
	}
	if cfg.Proxy.AdminListen == "" {
		cfg.Proxy.AdminListen = "127.0.0.1:9090"
	}
	// PACPath default handled in main() based on config file location
	if cfg.PAC.DNSTimeout == 0 {
		cfg.PAC.DNSTimeout = 500 * time.Millisecond
	}
	if cfg.PAC.EvalTimeout == 0 {
		cfg.PAC.EvalTimeout = 200 * time.Millisecond
	}
	if !cfg.PAC.EnableURLGlob {
		cfg.PAC.EnableURLGlob = false
	}
	return &cfg, nil
}

func discoverTUNIP(deviceName string) (net.IP, error) {
	iface, err := net.InterfaceByName(deviceName)
	if err != nil {
		return nil, err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
			return ipnet.IP, nil
		}
	}
	return nil, nil // no IPv4 found, not an error
}

// defaultConfigYAML returns the default config YAML with PAC path resolved.
func defaultConfigYAML(pacPath string) string {
	home, _ := os.UserHomeDir()
	defaultConfigDir := filepath.Join(home, ".config", "proxy-pac")
	return fmt.Sprintf(`proxy:
  http_listen: ":8080"
  socks5_listen: ":11080"
  admin_listen: "127.0.0.1:9090"
  https_listen: ":8443"
  pac_path: "%s"
  tun_device: "tun0"
  default_external_proxy: ""
  external_proxies: []
  tls:
    certificate_file: ""
    key_file: ""
    ca_file: ""
    min_version: 0
    max_version: 0
    cipher_suites: []
    prefer_server_cipher_suites: false
    session_tickets_disabled: false
    client_auth: ""
    client_cas: ""
    insecure_skip_verify: false
    timeout: 0s

pac:
  local_ip: ""
  dns_timeout: 500ms
  eval_timeout: 200ms
  enable_dns_cache: false
  enable_url_glob: false
  dns_max_entries: 100
  dns_ttl: 5m
  cache_enabled: false
  cache_max_size: 10000
  cache_ttl: 10m

gfw:
  disabled: false
  source_url: "https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt"
  local_rules_path: "%s"
  user_rules_path: "%s"
  use_tun: true
  update_interval: 24h
  fetch_on_startup: true
  http_timeout: 60s
  custom_dns_servers: []
`, pacPath, filepath.Join(defaultConfigDir, "gfw-rules.json"), filepath.Join(defaultConfigDir, "user-rules.json"))
}

// resolveConfigPath returns the config file path and whether it needs generation.
// Resolution priority: --conf flag > ~/.config/proxy-pac/config.yaml > auto-generate
func resolveConfigPath(confFlag string) (string, bool) {
	if confFlag != "" {
		return confFlag, false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Warning: could not determine home directory: %v", err)
		return "", false
	}
	defaultPath := filepath.Join(home, ".config", "proxy-pac", "config.yaml")
	if _, err := os.Stat(defaultPath); err == nil {
		return defaultPath, false
	}
	return defaultPath, true
}

// generateDefaultConfig creates a default config file at the given path.
func generateDefaultConfig(configPath string) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	pacPath := filepath.Join(filepath.Dir(configPath), "proxy.pac")
	if err := os.WriteFile(configPath, []byte(defaultConfigYAML(pacPath)), 0o644); err != nil {
		return err
	}
	log.Printf("Generated default config: %s", configPath)
	return nil
}

// defaultConfig returns a Config with all fields set to their defaults.
// PACPath is left empty — set in main() based on config file location.
func defaultConfig() *Config {
	cfg := &Config{}
	cfg.Proxy.HTTPListen = ":8080"
	cfg.Proxy.SOCKS5Listen = ":11080"
	cfg.Proxy.AdminListen = "127.0.0.1:9090"
	cfg.Proxy.HTTPSListen = ":8443"
	cfg.Proxy.TUNDevice = "tun0"
	cfg.PAC.DNSTimeout = 500 * time.Millisecond
	cfg.PAC.EvalTimeout = 200 * time.Millisecond
	return cfg
}

func main() {
	confFlag := flag.String("conf", "", "Path to config file")
	flag.Parse()

	configPath, needsGeneration := resolveConfigPath(*confFlag)

	if needsGeneration {
		log.Printf("No config found, generating default config")
		if err := generateDefaultConfig(configPath); err != nil {
			log.Printf("Warning: could not generate config at %s: %v", configPath, err)
		}
	}

	var cfg *Config
	var err error
	if configPath != "" {
		cfg, err = loadConfig(configPath)
		if err != nil {
			if *confFlag != "" {
				log.Fatalf("Failed to load config: %v", err)
			}
			log.Printf("Warning: could not load config at %s: %v", configPath, err)
			cfg = defaultConfig()
			log.Println("Using in-memory default config")
		}
	} else {
		cfg = defaultConfig()
		log.Println("Using in-memory default config")
	}

	// Default PAC path to be alongside the config file
	if cfg.Proxy.PACPath == "" {
		cfg.Proxy.PACPath = filepath.Join(filepath.Dir(configPath), "proxy.pac")
	}

	// Set default paths for user rules and GFW rules
	if cfg.GFW.UserRulesPath == "" {
		cfg.GFW.UserRulesPath = filepath.Join(filepath.Dir(configPath), "user-rules.json")
	}
	if cfg.GFW.LocalRulesPath == "" {
		cfg.GFW.LocalRulesPath = filepath.Join(filepath.Dir(configPath), "gfw-rules.json")
	}
	// Set default GFW source URL if not configured
	if cfg.GFW.SourceURL == "" {
		cfg.GFW.SourceURL = "https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt"
	}

	// Discover TUN IP
	tunIP, err := discoverTUNIP(cfg.Proxy.TUNDevice)
	if err != nil {
		log.Printf("Warning: could not discover TUN IP for %s: %v", cfg.Proxy.TUNDevice, err)
	}
	log.Printf("TUN device %s IP: %v", cfg.Proxy.TUNDevice, tunIP)

	// If LocalIP not set, use discovered TUN IP
	localIP := cfg.PAC.LocalIP
	if localIP == "" && tunIP != nil {
		localIP = tunIP.String()
	}

	// Create PAC evaluator
	evalCfg := pac.EvaluatorConfig{
		PACPath:        cfg.Proxy.PACPath,
		LocalIP:        localIP,
		DNSTimeout:     cfg.PAC.DNSTimeout,
		EvalTimeout:    cfg.PAC.EvalTimeout,
		EnableDNSCache: cfg.PAC.EnableDNSCache,
		CacheEnabled:   cfg.PAC.CacheEnabled,
		CacheMaxSize:   cfg.PAC.CacheMaxSize,
		CacheTTL:       cfg.PAC.CacheTTL,
		FallbackRoute:  pac.Route{Strategy: "DIRECT"},
	}
	evaluator, err := pac.NewEvaluator(evalCfg)
	if err != nil {
		log.Fatalf("Failed to create evaluator: %v", err)
	}
	defer evaluator.Close()

	// Create router
	routerCfg := pac.RouterConfig{
		TUNDevice:   cfg.Proxy.TUNDevice,
		TUNIP:       tunIP,
		DialTimeout: 30 * time.Second,
	}
	router, err := router.NewRouter(routerCfg)
	if err != nil {
		log.Fatalf("Failed to create router: %v", err)
	}

	// Create SSH tunnel manager if any SSH proxies are configured
	var tunnelMgr *ssh.Manager
	for _, p := range cfg.Proxy.ExternalProxies {
		if p.Kind == "ssh" {
			tunnelMgr = ssh.NewManager()
			break
		}
	}
	if tunnelMgr != nil {
		router.SetTunnelManager(tunnelMgr)
		// Ensure graceful shutdown
		defer func() {
			tunnelMgr.Close()
		}()
	}

	// Create and start proxy server
	proxyCfg := pac.ProxyConfig{
		HTTPListen:   cfg.Proxy.HTTPListen,
		SOCKS5Listen: cfg.Proxy.SOCKS5Listen,
		AdminListen:  cfg.Proxy.AdminListen,
		HTTPSListen:  cfg.Proxy.HTTPSListen,
		PACPath:      cfg.Proxy.PACPath,
		TUNDevice:    cfg.Proxy.TUNDevice,
		TLSConfig:    cfg.Proxy.TLS,
	}
	proxySrv := proxy.NewServer(proxyCfg, evaluator, router)
	proxySrv.SetTUNIP(tunIP)

	// Set external proxy registry and default
	if len(cfg.Proxy.ExternalProxies) > 0 {
		router.SetProxyRegistry(cfg.Proxy.ExternalProxies)
		// Note: SetProxyRegistry already calls tunnelMgr.UpdateProxies if tunnelMgr is set
	}
	if cfg.Proxy.DefaultExternalProxy != "" {
		router.SetDefaultProxyID(cfg.Proxy.DefaultExternalProxy)
	}

	// Create GFW fetcher if configured
	var gfwFetcher *gfw.Fetcher
	if cfg.GFW.SourceURL != "" {
		gfwPath := cfg.GFW.LocalRulesPath
		if gfwPath == "" {
			gfwPath = filepath.Join(filepath.Dir(configPath), "gfw-rules.json")
		}
		userPath := cfg.GFW.UserRulesPath
		if userPath == "" {
			userPath = filepath.Join(filepath.Dir(configPath), "user-rules.json")
		}
		httpTimeout := cfg.GFW.HTTPTimeout
		if httpTimeout == 0 {
			httpTimeout = 60 * time.Second
		}

		gfwFetcher = gfw.NewFetcher(cfg.GFW.SourceURL, gfwPath, httpTimeout)
		gfwFetcher.SetDisabled(cfg.GFW.Disabled)

		// IMPORTANT: Do NOT bind GFW fetcher to TUN interface to avoid DNS resolution issues
		// The GFW list must be fetched using the system's default network interface
		// to ensure proper DNS resolution of raw.githubusercontent.com
		if cfg.GFW.UseTUN && tunIP != nil {
			log.Printf("[GFW] TUN interface %s is configured for routing, but GFW fetcher uses system default for DNS resolution", tunIP)
			// Note: We intentionally do NOT bind the HTTP client to the TUN interface
			// to avoid DNS hijacking/resolution issues when fetching the GFW list
		}

		// Custom DNS: if configured, override HTTP client with custom resolver
		if len(cfg.GFW.CustomDNSServers) > 0 {
			resolver := pac.CreateCustomResolver(cfg.GFW.CustomDNSServers)
			dialer := &net.Dialer{
				Timeout:  httpTimeout,
				Resolver: resolver,
			}
			transport := &http.Transport{
				DialContext: dialer.DialContext,
			}
			customClient := &http.Client{
				Transport: transport,
				Timeout:   httpTimeout,
			}
			gfwFetcher.HTTPClient = customClient
			log.Printf("[GFW] Using custom DNS: %v", cfg.GFW.CustomDNSServers)
		}

		// Fetch on startup
		if cfg.GFW.FetchOnStartup && !gfwFetcher.IsDisabled() {
			go func() {
				log.Printf("[GFW] Fetching GFW list on startup...")
				result := gfwFetcher.Fetch(context.Background())
				if result.Error != nil {
					log.Printf("[GFW] Startup fetch failed: %v", result.Error)
				} else if result.Updated {
					log.Printf("[GFW] Startup fetch: %d rules loaded", result.RuleCount)
					regeneratePAC(userPath, gfwPath, cfg.Proxy.PACPath, cfg.Proxy.DefaultExternalProxy, cfg.PAC.EnableURLGlob, !gfwFetcher.IsDisabled())
				}
			}()
		}

		// Periodic fetch loop
		if cfg.GFW.UpdateInterval > 0 {
			go func() {
				ticker := time.NewTicker(cfg.GFW.UpdateInterval)
				defer ticker.Stop()
				for range ticker.C {
					if gfwFetcher.IsDisabled() {
						continue
					}
					log.Printf("[GFW] Periodic fetch...")
					result := gfwFetcher.Fetch(context.Background())
					if result.Error != nil {
						log.Printf("[GFW] Periodic fetch failed: %v", result.Error)
					} else if result.Updated {
						log.Printf("[GFW] Periodic fetch: %d rules updated", result.RuleCount)
						regeneratePAC(userPath, gfwPath, cfg.Proxy.PACPath, cfg.Proxy.DefaultExternalProxy, cfg.PAC.EnableURLGlob, !gfwFetcher.IsDisabled())
					}
				}
			}()
			log.Printf("[GFW] Periodic fetch enabled: every %v", cfg.GFW.UpdateInterval)
		}
	}

	// Start admin service
	adminMux := http.NewServeMux()
	adminHandler := admin.NewHandler(evaluator, cfg.Proxy.PACPath, gfwFetcher, cfg.GFW.UserRulesPath, cfg.GFW.LocalRulesPath, configPath, proxySrv, router, cfg.Proxy.DefaultExternalProxy)
	adminHandler.RegisterRoutes(adminMux)
	adminSrv := &http.Server{
		Addr:    cfg.Proxy.AdminListen,
		Handler: adminMux,
	}
	go func() {
		log.Printf("Admin UI listening on %s", cfg.Proxy.AdminListen)
		if err := adminSrv.ListenAndServe(); err != nil && !errorsIs(err, http.ErrServerClosed) {
			log.Printf("Admin server error: %v", err)
		}
	}()

	// Start proxy
	if err := proxySrv.Start(); err != nil {
		log.Fatalf("Proxy start failed: %v", err)
	}

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("Shutting down...")

	proxySrv.Stop()
	adminSrv.Shutdown(context.Background())
	evaluator.Close()
}

// errorsIs wrapper for Go 1.13+ but compatibility with older? We'll ignore
func errorsIs(err, target error) bool {
	return err == target
}

// regeneratePAC reads user and GFW rules, generates PAC, and writes it.
func regeneratePAC(userPath, gfwPath, pacPath string, defaultExternalProxy string, enableURLGlob bool, enabled bool) {
	if err := gfw.RegeneratePAC(userPath, gfwPath, pacPath, defaultExternalProxy, enableURLGlob, enabled); err != nil {
		log.Printf("[PAC] Regeneration failed: %v", err)
	}
}
