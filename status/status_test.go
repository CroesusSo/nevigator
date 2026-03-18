package status

import (
	"encoding/json"
	"testing"
	"time"
)

func TestConnectionStruct(t *testing.T) {
	now := time.Now()
	c := Connection{
		ID:         "conn-1",
		ClientAddr: "10.0.0.1:5234",
		DestAddr:   "example.com:443",
		Route:      "TUN",
		Protocol:   "http",
		StartedAt:  now,
	}
	if c.ID != "conn-1" {
		t.Errorf("ID = %q, want %q", c.ID, "conn-1")
	}
	if c.Route != "TUN" {
		t.Errorf("Route = %q, want %q", c.Route, "TUN")
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded Connection
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded.ID != c.ID || decoded.ClientAddr != c.ClientAddr || decoded.DestAddr != c.DestAddr {
		t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, c)
	}
}

func TestServerStatusStruct(t *testing.T) {
	s := ServerStatus{
		HTTPListen:   ":8080",
		SOCKS5Listen: ":1080",
		AdminListen:  "127.0.0.1:9090",
		TUNDevice:    "tun0",
		TUNIP:        "10.0.0.2",
		TUNUp:        true,
		ActiveConns:  5,
		TotalConns:   100,
	}
	if s.ActiveConns != 5 {
		t.Errorf("ActiveConns = %d, want 5", s.ActiveConns)
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded ServerStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded.TUNDevice != "tun0" || !decoded.TUNUp {
		t.Errorf("round-trip mismatch: got %+v", decoded)
	}
}

func TestServerStatusOmitEmpty(t *testing.T) {
	s := ServerStatus{
		HTTPListen: ":8080",
		TUNUp:      false,
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	// tunIP should be omitted when empty
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if _, ok := m["tunIP"]; ok {
		t.Error("tunIP should be omitted when empty")
	}
}

// mockProvider implements StatusProvider for testing.
type mockProvider struct {
	status ServerStatus
	conns  []Connection
}

func (m *mockProvider) Status() ServerStatus      { return m.status }
func (m *mockProvider) Connections() []Connection { return m.conns }

func TestStatusProviderInterface(t *testing.T) {
	var _ StatusProvider = &mockProvider{}
	mp := &mockProvider{
		status: ServerStatus{HTTPListen: ":8080", ActiveConns: 3},
		conns: []Connection{
			{ID: "conn-1", Route: "TUN"},
			{ID: "conn-2", Route: "DIRECT"},
		},
	}
	if mp.Status().ActiveConns != 3 {
		t.Errorf("ActiveConns = %d, want 3", mp.Status().ActiveConns)
	}
	if len(mp.Connections()) != 2 {
		t.Errorf("len(Connections) = %d, want 2", len(mp.Connections()))
	}
}
