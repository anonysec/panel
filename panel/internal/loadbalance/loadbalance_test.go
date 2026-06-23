//go:build !lite

package loadbalance

import (
	"math"
	"testing"
)

func TestCalculateLoad(t *testing.T) {
	tests := []struct {
		name     string
		active   int
		capacity int
		want     float64
	}{
		{"zero active", 0, 100, 0.0},
		{"half load", 50, 100, 50.0},
		{"full load", 100, 100, 100.0},
		{"over capacity", 120, 100, 120.0},
		{"zero capacity", 10, 0, 100.0},
		{"negative capacity", 10, -5, 100.0},
		{"small fraction", 1, 3, 100.0 / 3.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateLoad(tt.active, tt.capacity)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("CalculateLoad(%d, %d) = %f, want %f", tt.active, tt.capacity, got, tt.want)
			}
		})
	}
}

func TestSelectNode(t *testing.T) {
	tests := []struct {
		name      string
		nodes     []NodeLoad
		threshold float64
		wantID    int64
		wantErr   string
	}{
		{
			name:      "empty nodes",
			nodes:     []NodeLoad{},
			threshold: 90.0,
			wantID:    0,
			wantErr:   "no nodes available",
		},
		{
			name: "single node under threshold",
			nodes: []NodeLoad{
				{NodeID: 1, ActiveSessions: 10, MaxCapacity: 100},
			},
			threshold: 90.0,
			wantID:    1,
			wantErr:   "",
		},
		{
			name: "selects lowest load",
			nodes: []NodeLoad{
				{NodeID: 1, ActiveSessions: 80, MaxCapacity: 100},
				{NodeID: 2, ActiveSessions: 20, MaxCapacity: 100},
				{NodeID: 3, ActiveSessions: 50, MaxCapacity: 100},
			},
			threshold: 90.0,
			wantID:    2,
			wantErr:   "",
		},
		{
			name: "skips overloaded nodes",
			nodes: []NodeLoad{
				{NodeID: 1, ActiveSessions: 95, MaxCapacity: 100},
				{NodeID: 2, ActiveSessions: 91, MaxCapacity: 100},
				{NodeID: 3, ActiveSessions: 50, MaxCapacity: 100},
			},
			threshold: 90.0,
			wantID:    3,
			wantErr:   "",
		},
		{
			name: "all nodes overloaded",
			nodes: []NodeLoad{
				{NodeID: 1, ActiveSessions: 95, MaxCapacity: 100},
				{NodeID: 2, ActiveSessions: 92, MaxCapacity: 100},
				{NodeID: 3, ActiveSessions: 90, MaxCapacity: 100},
			},
			threshold: 90.0,
			wantID:    0,
			wantErr:   "all nodes overloaded",
		},
		{
			name: "node at exact threshold is excluded",
			nodes: []NodeLoad{
				{NodeID: 1, ActiveSessions: 90, MaxCapacity: 100},
				{NodeID: 2, ActiveSessions: 50, MaxCapacity: 100},
			},
			threshold: 90.0,
			wantID:    2,
			wantErr:   "",
		},
		{
			name: "zero capacity node treated as overloaded",
			nodes: []NodeLoad{
				{NodeID: 1, ActiveSessions: 5, MaxCapacity: 0},
				{NodeID: 2, ActiveSessions: 30, MaxCapacity: 100},
			},
			threshold: 90.0,
			wantID:    2,
			wantErr:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, err := SelectNode(tt.nodes, tt.threshold)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("expected error %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotID != tt.wantID {
				t.Errorf("SelectNode() = %d, want %d", gotID, tt.wantID)
			}
		})
	}
}

func TestIsOverloaded(t *testing.T) {
	tests := []struct {
		name      string
		node      NodeLoad
		threshold float64
		want      bool
	}{
		{
			name:      "under threshold",
			node:      NodeLoad{NodeID: 1, ActiveSessions: 50, MaxCapacity: 100},
			threshold: 90.0,
			want:      false,
		},
		{
			name:      "at threshold",
			node:      NodeLoad{NodeID: 1, ActiveSessions: 90, MaxCapacity: 100},
			threshold: 90.0,
			want:      true,
		},
		{
			name:      "above threshold",
			node:      NodeLoad{NodeID: 1, ActiveSessions: 95, MaxCapacity: 100},
			threshold: 90.0,
			want:      true,
		},
		{
			name:      "zero capacity is overloaded",
			node:      NodeLoad{NodeID: 1, ActiveSessions: 0, MaxCapacity: 0},
			threshold: 90.0,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsOverloaded(tt.node, tt.threshold)
			if got != tt.want {
				t.Errorf("IsOverloaded() = %v, want %v", got, tt.want)
			}
		})
	}
}
