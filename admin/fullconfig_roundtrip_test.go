package admin

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// Config keys not declared in FullConfig are dropped on any admin write.
func TestFullConfigPreservesSOCKS5AndKeepaliveKeys(t *testing.T) {
	src := `proxy:
  http_listen: ":8080"
  socks5_listen: ":1080"
  socks5_handshake_timeout: 7s
  socks5_auth:
    username: "alice"
    password: "s3cret"
  tcp_keepalive:
    enabled: true
    idle: 45s
    interval: 12s
    count: 4
  external_proxies:
    - id: "hk"
      kind: "ssh"
      address: "u@h:22"
      keepalive:
        enabled: true
        interval: 20s
        tcp:
          enabled: true
          idle: 25s
          interval: 8s
          count: 2
`
	var cfg FullConfig
	if err := yaml.Unmarshal([]byte(src), &cfg); err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(out)
	for _, want := range []string{
		"socks5_handshake_timeout",
		"socks5_auth",
		"username: alice",
		"tcp_keepalive",
		"idle: 45s",
		"count: 4",
	} {
		if !contains(got, want) {
			t.Errorf("admin write DROPS %q\n--- output ---\n%s", want, got)
		}
	}
}

func contains(h, n string) bool {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}

// End-to-end: a real admin write must not delete the new keys from disk.
func TestWriteFullConfigPreservesKeysOnDisk(t *testing.T) {
	src := `proxy:
  http_listen: ":8080"
  socks5_listen: ":1080"
  pac_path: "/tmp/x.pac"
  socks5_auth:
    username: "alice"
    password: "s3cret"
  socks5_handshake_timeout: 7s
  tcp_keepalive:
    enabled: true
    idle: 45s
    interval: 12s
    count: 4
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	h := &Handler{configPath: path}
	cfg, err := h.readConfig()
	if err != nil {
		t.Fatalf("readFullConfig: %v", err)
	}
	// Simulate an unrelated admin edit (e.g. toggling a listener).
	cfg.Proxy.SOCKS5Listen = ":1081"
	if err := h.writeFullConfig(*cfg); err != nil {
		t.Fatalf("writeFullConfig: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(after)
	for _, want := range []string{
		"socks5_listen: :1081",
		"socks5_auth",
		"username: alice",
		"password: s3cret",
		"socks5_handshake_timeout: 7s",
		"tcp_keepalive",
		"enabled: true",
		"idle: 45s",
		"interval: 12s",
		"count: 4",
	} {
		if !contains(got, want) {
			t.Errorf("after admin write, %q missing\n---\n%s", want, got)
		}
	}
	// And it must still parse as the real config type.
	var pc struct {
		Proxy struct {
			SOCKS5Auth *struct {
				Username string `yaml:"username"`
				Password string `yaml:"password"`
			} `yaml:"socks5_auth"`
		} `yaml:"proxy"`
	}
	if err := yaml.Unmarshal(after, &pc); err != nil {
		t.Fatalf("written config does not parse: %v", err)
	}
	if pc.Proxy.SOCKS5Auth == nil || pc.Proxy.SOCKS5Auth.Username != "alice" {
		t.Errorf("socks5_auth did not survive: %#v", pc.Proxy.SOCKS5Auth)
	}
}
