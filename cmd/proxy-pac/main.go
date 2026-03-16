package main

import (
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
	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/proxy"
	"github.com/yourorg/proxy-pac/router"
	"gopkg.in/yaml.v3"
)

type Config struct {
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
	if cfg.Proxy.TUNDevice == "" {
		cfg.Proxy.TUNDevice = "tun0"
	}
	if cfg.PAC.DNSTimeout == 0 {
		cfg.PAC.DNSTimeout = 500 * time.Millisecond
	}
	if cfg.PAC.EvalTimeout == 0 {
		cfg.PAC.EvalTimeout = 200 * time.Millisecond
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
	return fmt.Sprintf(`proxy:
  http_listen: ":8080"
  socks5_listen: ":1080"
  admin_listen: "127.0.0.1:9090"
  pac_path: "%s"
  tun_device: "tun0"

pac:
  local_ip: ""
  dns_timeout: 500ms
  eval_timeout: 200ms
  enable_dns_cache: false
`, pacPath)
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
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}
	pacPath := filepath.Join(filepath.Dir(configPath), "proxy.pac")
	if err := os.WriteFile(configPath, []byte(defaultConfigYAML(pacPath)), 0644); err != nil {
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
	cfg.Proxy.SOCKS5Listen = ":1080"
	cfg.Proxy.AdminListen = "127.0.0.1:9090"
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

	// Create and start proxy server
	proxyCfg := pac.ProxyConfig{
		HTTPListen:   cfg.Proxy.HTTPListen,
		SOCKS5Listen: cfg.Proxy.SOCKS5Listen,
		AdminListen:  cfg.Proxy.AdminListen,
		PACPath:      cfg.Proxy.PACPath,
		TUNDevice:    cfg.Proxy.TUNDevice,
	}
	proxySrv := proxy.NewServer(proxyCfg, evaluator, router)

	// Start admin service
	adminMux := http.NewServeMux()
	adminHandler := admin.NewHandler(evaluator, cfg.Proxy.PACPath)
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
	adminSrv.Shutdown(nil)
	evaluator.Close()
}

// errorsIs wrapper for Go 1.13+ but compatibility with older? We'll ignore
func errorsIs(err, target error) bool {
	return err == target
}
