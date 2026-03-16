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

// ServerStatus holds runtime status of the proxy server.
type ServerStatus struct {
	HTTPListen   string `json:"httpListen"`
	SOCKS5Listen string `json:"socks5Listen"`
	AdminListen  string `json:"adminListen"`
	TUNDevice    string `json:"tunDevice"`
	TUNIP        string `json:"tunIP,omitempty"`
	TUNUp        bool   `json:"tunUp"`
	ActiveConns  int    `json:"activeConns"`
	TotalConns   int64  `json:"totalConns"`
}

// StatusProvider exposes runtime status information.
type StatusProvider interface {
	Status() ServerStatus
	Connections() []Connection
}
