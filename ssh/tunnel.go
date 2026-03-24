package ssh

import (
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yourorg/proxy-pac/pac"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var (
	ErrTunnelClosed = fmt.Errorf("tunnel closed")
	ErrPermanent    = fmt.Errorf("permanent failure")

	// dialSSHVal holds the function used to establish SSH connections.
	// It is an atomic.Value to allow safe replacement in tests.
	dialSSHVal atomic.Value // type: func(pac.ExternalProxy) (*ssh.Client, error)
)

func init() {
	// Initialize with the real dialSSH function
	dialSSHVal.Store(dialSSH)
}

// getDialSSH returns the current dial function (for test override support)
func getDialSSH() func(pac.ExternalProxy) (*ssh.Client, error) {
	return dialSSHVal.Load().(func(pac.ExternalProxy) (*ssh.Client, error))
}

const (
	stateClosed       = "closed"
	stateConnecting   = "connecting"
	stateConnected    = "connected"
	stateReconnecting = "reconnecting"
)

// Tunnel represents a single SSH tunnel connection to a proxy.
type Tunnel struct {
	mu            sync.Mutex
	cfg           pac.ExternalProxy
	client        *ssh.Client
	state         string
	backoff       time.Duration
	closing       bool
	connectedChan chan struct{} // closed to broadcast connected state
	lastErr       error
	proxyID       string
}

// NewTunnel creates a new Tunnel with the given configuration.
func NewTunnel(cfg pac.ExternalProxy) *Tunnel {
	t := &Tunnel{
		cfg:           cfg,
		state:         stateClosed,
		proxyID:       cfg.ID,
		connectedChan: make(chan struct{}),
	}
	return t
}

// broadcast closes the current connectedChan and replaces it with a fresh one,
// waking all waiters. It must be called with the mutex unlocked (it locks internally).
func (t *Tunnel) broadcast() {
	t.mu.Lock()
	old := t.connectedChan
	t.connectedChan = make(chan struct{})
	t.mu.Unlock()
	close(old)
}

// Dial establishes a connection through the tunnel to the destination.
// It blocks until the tunnel is connected and the channel is opened, or an error occurs.
// The timeout parameter bounds the total time including tunnel establishment.
func (t *Tunnel) Dial(dest string, timeout time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(timeout)
	for {
		t.mu.Lock()
		if t.closing {
			t.mu.Unlock()
			return nil, ErrTunnelClosed
		}

		// Start connection if not already connected
		if t.state == stateClosed {
			t.state = stateConnecting
			go t.connectLoop()
		}

		// Check if already connected
		if t.client != nil && t.state == stateConnected {
			t.mu.Unlock()
			conn, err := t.client.Dial("tcp", dest)
			if err != nil {
				t.mu.Lock()
				t.client.Close()
				t.client = nil
				t.state = stateReconnecting
				t.lastErr = err
				t.mu.Unlock()
				t.broadcast()
				continue
			}
			return conn, nil
		}

		if isPermanentError(t.lastErr) {
			t.mu.Unlock()
			t.broadcast()
			return nil, t.lastErr
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.mu.Unlock()
			return nil, fmt.Errorf("dial timeout: %w", t.lastErr)
		}

		// Capture current notifier
		notifier := t.connectedChan
		t.mu.Unlock()

		select {
		case <-notifier:
			continue
		case <-time.After(remaining):
			// Final check after timeout
			t.mu.Lock()
			if t.client != nil && t.state == stateConnected {
				t.mu.Unlock()
				conn, err := t.client.Dial("tcp", dest)
				if err != nil {
					t.mu.Lock()
					t.client.Close()
					t.client = nil
					t.state = stateReconnecting
					t.lastErr = err
					t.mu.Unlock()
					t.broadcast()
					continue
				}
				return conn, nil
			}
			t.mu.Unlock()
			if t.lastErr != nil {
				return nil, fmt.Errorf("dial timeout: %w", t.lastErr)
			}
			return nil, fmt.Errorf("dial timeout")
		}
	}
}

// connectLoop runs in a goroutine and attempts to establish the SSH connection.
func (t *Tunnel) connectLoop() {
	for {
		t.mu.Lock()
		if t.closing {
			t.mu.Unlock()
			return
		}
		cfg := t.cfg
		t.state = stateConnecting
		t.lastErr = nil
		t.mu.Unlock()

		log.Printf("[SSH] Tunnel %s connecting to %s", t.proxyID, cfg.Address)
		client, err := getDialSSH()(cfg)
		t.mu.Lock()
		if t.closing {
			if client != nil {
				client.Close()
			}
			t.mu.Unlock()
			return
		}

		if err != nil {
			t.client = nil
			t.state = stateReconnecting
			t.lastErr = err
			log.Printf("[SSH] Tunnel %s connection failed: %v", t.proxyID, err)
			if isPermanentError(err) {
				t.closing = true
			}
			t.mu.Unlock()
			t.broadcast()
			if isPermanentError(err) {
				return
			}
			backoff := t.computeBackoff()
			log.Printf("[SSH] Tunnel %s reconnecting in %v", t.proxyID, backoff)
			time.Sleep(backoff)
			continue
		}

		// Success
		t.client = client
		t.backoff = 0
		t.state = stateConnected
		t.lastErr = nil
		log.Printf("[SSH] Tunnel %s connected", t.proxyID)
		t.mu.Unlock()
		t.broadcast()
		return
	}
}

// computeBackoff returns exponential backoff with jitter.
func (t *Tunnel) computeBackoff() time.Duration {
	backoff := t.backoff
	if backoff == 0 {
		backoff = 1 * time.Second
	} else {
		backoff = backoff * 2
	}
	if backoff > 5*time.Minute {
		backoff = 5 * time.Minute
	}
	// Jitter ±20%
	jitter := time.Duration(float64(backoff) * (0.8 + 0.4*float64(time.Now().UnixNano()%1000)/1000.0))
	t.backoff = jitter
	return jitter
}

// Close shuts down the tunnel.
func (t *Tunnel) Close() error {
	t.mu.Lock()
	if t.closing {
		t.mu.Unlock()
		return nil
	}
	t.closing = true
	if t.client != nil {
		t.client.Close()
		t.client = nil
	}
	t.state = stateClosed
	log.Printf("[SSH] Tunnel %s closed", t.proxyID)
	t.mu.Unlock()
	t.broadcast()
	return nil
}

// isPermanentError returns true for errors that should stop retrying.
func isPermanentError(err error) bool {
	if err == nil {
		return false
	}
	// Check error message for permanent failures
	s := err.Error()
	if strings.Contains(s, "password denied") ||
		strings.Contains(s, "public key denied") ||
		strings.Contains(s, "host key verification failed") ||
		strings.Contains(s, "permission denied") ||
		strings.Contains(s, "no such file or directory") ||
		strings.Contains(s, "invalid private key") {
		return true
	}
	return false
}

// dialSSH establishes an SSH connection using the provided proxy configuration.
func dialSSH(cfg pac.ExternalProxy) (*ssh.Client, error) {
	host, port, username, err := parseSSHAddress(cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}
	// Override with explicit fields if provided
	if cfg.Username != "" {
		username = cfg.Username
	}
	if port == 0 {
		if cfg.SSHPort != 0 {
			port = cfg.SSHPort
		} else {
			port = 22
		}
	}

	sshCfg := &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{},
		HostKeyCallback: makeHostKeyCallback(cfg),
		Timeout:         30 * time.Second,
	}

	switch cfg.AuthMethod {
	case "key":
		if cfg.PrivateKeyPath == "" {
			return nil, fmt.Errorf("private_key_path required for key authentication")
		}
		// Check file permissions (warn if world-readable)
		checkKeyFilePermissions(cfg.PrivateKeyPath)
		key, err := os.ReadFile(cfg.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("read private key: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		sshCfg.Auth = append(sshCfg.Auth, ssh.PublicKeys(signer))
	case "password":
		if cfg.Password == "" {
			return nil, fmt.Errorf("password required for password authentication")
		}
		sshCfg.Auth = append(sshCfg.Auth, ssh.Password(cfg.Password))
	default:
		return nil, fmt.Errorf("invalid auth_method: %s", cfg.AuthMethod)
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	return ssh.Dial("tcp", addr, sshCfg)
}

// dialSSHFunc is the function used by connectLoop to establish an SSH connection.
// It defaults to dialSSH but can be overridden in tests.
var dialSSHFunc = dialSSH

// checkKeyFilePermissions warns if the key file has overly permissive permissions.
func checkKeyFilePermissions(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return // will fail later on read
	}
	mode := info.Mode()
	// On Unix, check if any permission bits outside owner exist
	if mode&0o077 != 0 {
		log.Printf("[SSH] Warning: private key file %s has permissive permissions (%04o); recommended 0600", path, mode&0o777)
	}
}

// parseSSHAddress parses strings like "host", "host:port", "user@host", "user@host:port"
// Returns host, port, username. Port is 0 if not specified.
func parseSSHAddress(addr string) (host string, port int, user string, err error) {
	if at := strings.Index(addr, "@"); at != -1 {
		user = addr[:at]
		addr = addr[at+1:]
	}

	if host, portStr, err := net.SplitHostPort(addr); err == nil {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			return "", 0, "", fmt.Errorf("invalid port: %w", err)
		}
		return host, p, user, nil
	}
	return addr, 0, user, nil
}

// makeHostKeyCallback returns a HostKeyCallback based on configuration.
func makeHostKeyCallback(cfg pac.ExternalProxy) ssh.HostKeyCallback {
	switch cfg.HostKeyCheck {
	case "ignore":
		return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		}
	case "strict", "":
		if cfg.HostKeyFingerprint != "" {
			want := strings.ToLower(strings.ReplaceAll(cfg.HostKeyFingerprint, ":", ""))
			return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
				got := strings.ToLower(hex.EncodeToString(key.Marshal()))
				if got != want {
					return fmt.Errorf("host key fingerprint mismatch: want %s, got %s", want, got)
				}
				return nil
			}
		}
		knownhostsPath := defaultKnownHostsPath()
		cb, err := knownhosts.New(knownhostsPath)
		if err != nil {
			log.Printf("[SSH] known_hosts at %s: %v; defaulting to no verification", knownhostsPath, err)
			return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
				return nil
			}
		}
		return cb
	default:
		log.Printf("[SSH] unknown host_key_check: %s; defaulting to ignore", cfg.HostKeyCheck)
		return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		}
	}
}

// defaultKnownHostsPath returns the path to the system known_hosts file.
func defaultKnownHostsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssh", "known_hosts")
}
