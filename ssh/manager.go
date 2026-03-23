package ssh

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/yourorg/proxy-pac/pac"
	"github.com/yourorg/proxy-pac/status"
)

// Manager manages SSH tunnels for a set of configured proxies.
type Manager struct {
	mu      sync.RWMutex
	proxies map[string]pac.ExternalProxy // config snapshot
	tunnels map[string]*Tunnel           // active tunnels by proxy ID
}

// NewManager creates a new Manager.
func NewManager() *Manager {
	return &Manager{
		proxies: make(map[string]pac.ExternalProxy),
		tunnels: make(map[string]*Tunnel),
	}
}

// UpdateProxies atomically updates the proxy configuration.
// It closes tunnels that are no longer present and adds new ones.
func (m *Manager) UpdateProxies(proxies []pac.ExternalProxy) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build new proxy map
	newMap := make(map[string]pac.ExternalProxy, len(proxies))
	for _, p := range proxies {
		if p.Kind != "ssh" {
			continue
		}
		newMap[p.ID] = p
	}

	// Close tunnels that are no longer present
	for id, tunnel := range m.tunnels {
		if _, ok := newMap[id]; !ok {
			tunnel.Close()
			delete(m.tunnels, id)
			log.Printf("[SSH] Tunnel %s removed", id)
		}
	}

	// Update config map
	m.proxies = newMap
}

// Dial returns a connection through the SSH proxy with the given ID to the destination.
// It respects the timeout. If the tunnel is not yet connected, Dial blocks until it connects
// or fails permanently.
func (m *Manager) Dial(id, dest string, timeout time.Duration) (net.Conn, error) {
	// Get or create tunnel
	tunnel := m.getOrCreateTunnel(id)
	return tunnel.Dial(dest, timeout)
}

// getOrCreateTunnel returns the tunnel for the given proxy ID, creating it if needed.
func (m *Manager) getOrCreateTunnel(id string) *Tunnel {
	m.mu.RLock()
	cfg, ok := m.proxies[id]
	m.mu.RUnlock()
	if !ok {
		// Return a nil tunnel that always fails
		return &Tunnel{
			cfg:     pac.ExternalProxy{ID: id},
			state:   stateClosed,
			closing: true,
			lastErr: fmt.Errorf("SSH proxy %s not configured", id),
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	tunnel, exists := m.tunnels[id]
	if !exists {
		tunnel = NewTunnel(cfg)
		m.tunnels[id] = tunnel
	}
	return tunnel
}

// Close closes all tunnels and releases resources.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, tunnel := range m.tunnels {
		tunnel.Close()
		delete(m.tunnels, id)
	}
	return nil
}

// GetTunnelsStatus returns a snapshot of all tunnels' runtime status.
func (m *Manager) GetTunnelsStatus() map[string]status.SSHTunnelStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]status.SSHTunnelStatus, len(m.tunnels))
	for id, tunnel := range m.tunnels {
		tunnel.mu.Lock()
		state := tunnel.state
		lastErr := ""
		if tunnel.lastErr != nil {
			lastErr = tunnel.lastErr.Error()
		}
		remoteAddr := ""
		if tunnel.client != nil {
			remoteAddr = tunnel.client.RemoteAddr().String()
		} else {
			remoteAddr = tunnel.cfg.Address
		}
		tunnel.mu.Unlock()
		result[id] = status.SSHTunnelStatus{
			State:      state,
			LastError:  lastErr,
			RemoteAddr: remoteAddr,
		}
	}
	return result
}
