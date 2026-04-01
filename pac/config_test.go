package pac

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestKeepaliveConfig_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		cfg      KeepaliveConfig
		expected string
	}{
		{
			name: "enabled with interval",
			cfg: KeepaliveConfig{
				Enabled:  true,
				Interval: 30 * time.Second,
			},
			expected: `{"enabled":true,"interval":"30s"}`,
		},
		{
			name: "enabled with zero interval",
			cfg: KeepaliveConfig{
				Enabled:  true,
				Interval: 0,
			},
			expected: `{"enabled":true}`,
		},
		{
			name: "disabled with interval",
			cfg: KeepaliveConfig{
				Enabled:  false,
				Interval: 30 * time.Second,
			},
			expected: `{"enabled":false,"interval":"30s"}`,
		},
		{
			name: "disabled with zero interval",
			cfg: KeepaliveConfig{
				Enabled:  false,
				Interval: 0,
			},
			expected: `{"enabled":false}`,
		},
		{
			name: "one minute interval",
			cfg: KeepaliveConfig{
				Enabled:  true,
				Interval: time.Minute,
			},
			expected: `{"enabled":true,"interval":"1m0s"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.cfg.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON failed: %v", err)
			}
			// Compare as JSON objects to ignore field order
			var m1, m2 map[string]interface{}
			if err := json.Unmarshal(data, &m1); err != nil {
				t.Fatalf("Unmarshal result: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.expected), &m2); err != nil {
				t.Fatalf("Unmarshal expected: %v", err)
			}
			if !reflect.DeepEqual(m1, m2) {
				t.Fatalf("expected %s, got %s", tt.expected, string(data))
			}
		})
	}
}
