package status

import "time"

// Connection represents an active proxy connection.
type Connection struct {
	ID         string    `json:"id"`
	ClientAddr string    `json:"clientAddr"`
	DestAddr   string    `json:"destAddr"`
	Route      string    `json:"route"`
	Protocol   string    `json:"protocol"`
	StartedAt  time.Time `json:"startedAt"`
}

// SSHTunnelStatus represents the runtime status of an SSH tunnel.
type SSHTunnelStatus struct {
	State      string `json:"state"` // "connected", "connecting", "reconnecting", "closed"
	LastError  string `json:"lastError,omitempty"`
	RemoteAddr string `json:"remoteAddr"` // SSH server address
}

// ServerStatus holds runtime status of the proxy server.
type ServerStatus struct {
	HTTPListen   string                     `json:"httpListen"`
	SOCKS5Listen string                     `json:"socks5Listen"`
	AdminListen  string                     `json:"adminListen"`
	HTTPSListen  string                     `json:"httpsListen,omitempty"`
	ActiveConns  int                        `json:"activeConns"`
	TotalConns   int64                      `json:"totalConns"`
	SSHTunnels   map[string]SSHTunnelStatus `json:"sshTunnels,omitempty"`
}

// StatusProvider exposes runtime status information.
type StatusProvider interface {
	Status() ServerStatus
	Connections() []Connection
}
