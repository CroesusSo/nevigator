// Package ssh provides SSH tunnel support for the proxy.
//
// Tunnel uses a manager goroutine and a worker pool to handle concurrent Dial
// requests with zero lock contention in the hot path. The manager owns the SSH
// client and handles (re)connection. Workers retrieve the client atomically and
// perform Dial. Failures signal the manager via a non-blocking channel. Workers
// listen on context cancellation to exit gracefully, eliminating the need for
// channel closing. A condition variable coordinates workers waiting for the
// initial connection. This design provides high throughput under load.
package ssh

import (
	"context"
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
	ErrOverload     = fmt.Errorf("tunnel overloaded")

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
	stateClosed     = "closed"
	stateConnecting = "connecting"
	stateConnected  = "connected"
)

// dialTask represents a request to dial through the tunnel.
type dialTask struct {
	dest     string
	respChan chan<- dialResult
}

// dialResult is the result of a dial attempt.
type dialResult struct {
	conn net.Conn
	err  error
}

// Tunnel represents a single SSH tunnel connection to a proxy.
// Uses a worker pool pattern with atomic state to eliminate lock contention.
type Tunnel struct {
	cfg     pac.ExternalProxy
	proxyID string

	// Atomic state (accessed via atomics)
	client  atomic.Pointer[ssh.Client]
	state   atomic.Value // string
	closing atomic.Bool

	// Channels
	dialCh chan dialTask // buffered task queue
	failCh chan struct{} // signals from workers that connection failed
	ctx    context.Context
	cancel context.CancelFunc

	// Condition variable for workers to wait for connection
	cond *sync.Cond

	// Worker management
	wg      sync.WaitGroup
	backoff time.Duration // manager-only (protected by single-manager goroutine)

	// Keepalive management
	keepaliveCtx      context.Context
	keepaliveCancel   context.CancelFunc
	keepaliveFailures int32 // atomic access
}

// NewTunnel creates a new Tunnel with the given configuration.
func NewTunnel(cfg pac.ExternalProxy) *Tunnel {
	const (
		defaultWorkers = 8
		dialBufSize    = 100
		failBufSize    = 10
	)

	ctx, cancel := context.WithCancel(context.Background())
	t := &Tunnel{
		cfg:     cfg,
		proxyID: cfg.ID,
		dialCh:  make(chan dialTask, dialBufSize),
		failCh:  make(chan struct{}, failBufSize),
		ctx:     ctx,
		cancel:  cancel,
		cond:    sync.NewCond(&sync.Mutex{}),
	}
	t.state.Store(stateClosed)

	// Start workers
	for i := 0; i < defaultWorkers; i++ {
		t.wg.Add(1)
		go t.worker()
	}

	// Start manager
	t.wg.Add(1)
	go t.manager()

	return t
}

// manager runs the connection lifecycle. It attempts initial connection and
// listens for failCh signals to trigger reconnection. It does NOT process dial tasks.
func (t *Tunnel) manager() {
	defer t.wg.Done()

	// Initial connection attempt
	t.connect()

	// Listen for failure signals or shutdown
	for {
		select {
		case <-t.failCh:
			// Any failure signals the need to (re)connect.
			// The connect() method is idempotent and will establish connection if not already connecting.
			t.connect()
		case <-t.ctx.Done():
			return
		}
	}
}

// isConnected is a fast lock-free check of connection state.
func (t *Tunnel) isConnected() bool {
	s := t.state.Load().(string)
	client := t.client.Load()
	return s == stateConnected && client != nil
}

// connect attempts to establish or re-establish the SSH connection, with exponential backoff on failures.
// It runs in the manager goroutine only.
func (t *Tunnel) connect() {
	// If an attempt is already in progress, skip. We may be called from "connected" state to trigger reconnection.
	current := t.state.Load().(string)
	if current == stateConnecting {
		return
	}

	for {
		// Check for shutdown or permanent closure
		if t.closing.Load() {
			t.setState(stateClosed)
			return
		}

		t.setState(stateConnecting)
		log.Printf("[SSH] Tunnel %s connecting to %s", t.proxyID, t.cfg.Address)

		client, err := getDialSSH()(t.cfg)
		if err != nil {
			if isPermanentError(err) {
				t.closing.Store(true)
				t.setState(stateClosed)
				log.Printf("[SSH] Tunnel %s permanent failure: %v", t.proxyID, err)
				return
			}

			// Transient error: wait with exponential backoff
			backoff := t.computeBackoff()
			log.Printf("[SSH] Tunnel %s connection failed: %v; reconnecting in %v", t.proxyID, err, backoff)

			// Wait with backoff, but respect context cancellation
			select {
			case <-time.After(backoff):
				// continue retrying
			case <-t.ctx.Done():
				t.setState(stateClosed)
				return
			}
			continue
		}

		// Success: atomically install new client
		t.setClient(client)
		t.setState(stateConnected)
		t.backoff = 0
		// Wake up any workers waiting for a connection
		t.cond.Broadcast()
		log.Printf("[SSH] Tunnel %s connected", t.proxyID)
		return
	}
}

// setClient atomically replaces the current SSH client, closing any existing one.
func (t *Tunnel) setClient(newClient *ssh.Client) {
	// Stop keepalive on old connection before swap
	t.stopKeepalive()

	old := t.client.Swap(newClient)
	if old != nil {
		old.Close()
	}

	// Start keepalive on new connection if enabled
	if t.cfg.Keepalive.Enabled && newClient != nil {
		atomic.StoreInt32(&t.keepaliveFailures, 0) // reset counter
		// Apply default interval if not set
		if t.cfg.Keepalive.Interval == 0 {
			t.cfg.Keepalive.Interval = 30 * time.Second
		}
		t.startKeepalive(newClient)
	}
}

// setState updates the atomic state string.
func (t *Tunnel) setState(s string) {
	t.state.Store(s)
}

// keepaliveLoop runs in its own goroutine and sends periodic keepalive probes.
func (t *Tunnel) keepaliveLoop(ctx context.Context, client *ssh.Client) {
	ticker := time.NewTicker(t.cfg.Keepalive.Interval)
	defer ticker.Stop()

	// Ping timeout: 5 seconds
	pingTimeout := 5 * time.Second
	// Max consecutive failures before reconnect
	const maxFailures = 3

	for {
		select {
		case <-ctx.Done():
			// Stop requested
			return
		case <-ticker.C:
			// Create child context for this ping with timeout
			pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
			errCh := make(chan error, 1)

			// Launch goroutine to send keepalive (this may block)
			go func() {
				_, _, err := client.SendRequest("keepalive@golang.org", true, nil)
				errCh <- err
			}()

			var err error
			select {
			case err = <-errCh:
				// Keepalive response received (success or error)
			case <-pingCtx.Done():
				// Ping timed out
				err = pingCtx.Err()
			}
			cancel() // Clean up child context

			if err != nil {
				log.Printf("[SSH] Tunnel %s keepalive failed: %v", t.proxyID, err)
				atomic.AddInt32(&t.keepaliveFailures, 1)
				failures := atomic.LoadInt32(&t.keepaliveFailures)
				if failures >= maxFailures {
					log.Printf("[SSH] Tunnel %s keepalive failed %d times; closing connection", t.proxyID, failures)
					// Mark tunnel as broken: swap client to nil, close old, signal manager
					old := t.client.Swap(nil)
					if old != nil {
						old.Close()
					}
					select {
					case t.failCh <- struct{}{}:
					default:
					}
					return
				}
			} else {
				// Success: reset failure counter
				atomic.StoreInt32(&t.keepaliveFailures, 0)
			}
		}
	}
}

// startKeepalive begins the keepalive goroutine for the current client.
func (t *Tunnel) startKeepalive(client *ssh.Client) {
	if t.cfg.Keepalive.Enabled && t.cfg.Keepalive.Interval > 0 {
		t.keepaliveCtx, t.keepaliveCancel = context.WithCancel(t.ctx)
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.keepaliveLoop(t.keepaliveCtx, client)
		}()
		log.Printf("[SSH] Tunnel %s keepalive started (interval: %v)", t.proxyID, t.cfg.Keepalive.Interval)
	}
}

// stopKeepalive stops the keepalive goroutine if running.
func (t *Tunnel) stopKeepalive() {
	if t.keepaliveCancel != nil {
		t.keepaliveCancel()
		t.keepaliveCancel = nil
	}
}

// computeBackoff returns exponential backoff with jitter and updates t.backoff.
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

// worker processes dial tasks from dialCh. It reads client atomically and
// sends a non-blocking signal to failCh on errors.
func (t *Tunnel) worker() {
	defer t.wg.Done()
	for {
		select {
		case task := <-t.dialCh:
			// Check if tunnel is closing
			if t.closing.Load() {
				task.respChan <- dialResult{err: ErrTunnelClosed}
				continue
			}

			// Get current client atomically
			client := t.client.Load()
			if client == nil {
				// Wait for connection to be established or closing
				t.cond.L.Lock()
				for t.client.Load() == nil && !t.closing.Load() {
					t.cond.Wait()
				}
				t.cond.L.Unlock()
				client = t.client.Load()
				if client == nil || t.closing.Load() {
					task.respChan <- dialResult{err: ErrTunnelClosed}
					continue
				}
			}

			// Perform the dial
			conn, err := client.Dial("tcp", task.dest)
			if err != nil {
				// Mark tunnel as broken: swap client to nil and close old
				old := t.client.Swap(nil)
				if old != nil {
					old.Close()
				}
				// Signal manager to reconnect (non-blocking)
				select {
				case t.failCh <- struct{}{}:
				default:
				}
				task.respChan <- dialResult{err: err}
				continue
			}

			// Success
			task.respChan <- dialResult{conn: conn, err: nil}
		case <-t.ctx.Done():
			return
		}
	}
}

// Dial establishes a connection through the tunnel to the destination.
// It blocks until the connection is established or an error occurs, respecting timeout.
// If the internal task queue is full, it returns ErrOverload immediately.
func (t *Tunnel) Dial(dest string, timeout time.Duration) (net.Conn, error) {
	// Build task
	respChan := make(chan dialResult, 1)
	task := dialTask{
		dest:     dest,
		respChan: respChan,
	}

	// Fast check: if closing, reject immediately
	if t.closing.Load() {
		return nil, ErrTunnelClosed
	}

	// Non-blocking enqueue with backpressure
	select {
	case t.dialCh <- task:
		// queued
	default:
		return nil, ErrOverload
	}

	// Wait for result or timeout
	deadline := time.Now().Add(timeout)
	for {
		select {
		case result := <-respChan:
			if result.err != nil {
				return nil, result.err
			}
			return result.conn, nil
		case <-time.After(time.Until(deadline)):
			return nil, fmt.Errorf("dial timeout")
		}
	}
}

// Close shuts down the tunnel gracefully. It is safe to call multiple times.
func (t *Tunnel) Close() error {
	// Use CAS to ensure only one Close proceeds
	if !t.closing.CompareAndSwap(false, true) {
		return nil // already closing/closed
	}

	// Wake any workers waiting for a connection
	t.cond.Broadcast()

	// Cancel manager and workers (they will exit upon ctx.Done())
	t.cancel()

	// Stop keepalive explicitly to clean up
	t.stopKeepalive()

	// Wait for all goroutines to exit
	t.wg.Wait()

	// Close the underlying SSH client if any
	if client := t.client.Load(); client != nil {
		client.Close()
		t.client.Store(nil)
	}

	// Clear state
	t.setState(stateClosed)
	log.Printf("[SSH] Tunnel %s closed", t.proxyID)

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

	// Use custom dialer with TCP keepalive
	dialer := &net.Dialer{
		Timeout:   sshCfg.Timeout,
		KeepAlive: 30 * time.Second,
	}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("tcp dial failed: %w", err)
	}

	// Perform SSH handshake on the connection
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, sshCfg)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ssh handshake failed: %w", err)
	}

	// Wrap into ssh.Client
	client := ssh.NewClient(sshConn, chans, reqs)
	return client, nil
}

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
