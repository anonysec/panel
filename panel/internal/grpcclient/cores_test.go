package grpcclient

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsConcurrentModification(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "AlreadyExists is concurrent modification",
			err:      status.Error(codes.AlreadyExists, "core already enabled"),
			expected: true,
		},
		{
			name:     "NotFound is concurrent modification",
			err:      status.Error(codes.NotFound, "core not found"),
			expected: true,
		},
		{
			name:     "FailedPrecondition is concurrent modification",
			err:      status.Error(codes.FailedPrecondition, "core in transitional state"),
			expected: true,
		},
		{
			name:     "Internal is not concurrent modification",
			err:      status.Error(codes.Internal, "unexpected failure"),
			expected: false,
		},
		{
			name:     "Unavailable is not concurrent modification",
			err:      status.Error(codes.Unavailable, "node unreachable"),
			expected: false,
		},
		{
			name:     "PermissionDenied is not concurrent modification",
			err:      status.Error(codes.PermissionDenied, "bad credentials"),
			expected: false,
		},
		{
			name:     "DeadlineExceeded is not concurrent modification",
			err:      status.Error(codes.DeadlineExceeded, "timeout"),
			expected: false,
		},
		{
			name:     "ResourceExhausted is not concurrent modification",
			err:      status.Error(codes.ResourceExhausted, "rate limited"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isConcurrentModification(tt.err)
			if got != tt.expected {
				t.Errorf("isConcurrentModification(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestRefreshNodeState_NodeNotInPool(t *testing.T) {
	pool := &mockPoolForCores{nodes: map[int64]*NodeConnection{}}
	err := RefreshNodeState(context.Background(), pool, nil, 999)
	if err == nil {
		t.Error("expected error when node is not in pool, got nil")
	}
}

func TestRefreshNodeState_NodeOffline(t *testing.T) {
	pool := &mockPoolForCores{
		nodes: map[int64]*NodeConnection{
			1: {
				NodeID:   1,
				NodeName: "offline-node",
				Status:   StatusOffline,
			},
		},
	}
	err := RefreshNodeState(context.Background(), pool, nil, 1)
	if err == nil {
		t.Error("expected error when node is offline, got nil")
	}
}

// mockPoolForCores implements Pool for testing cores.go functionality.
type mockPoolForCores struct {
	nodes map[int64]*NodeConnection
}

func (m *mockPoolForCores) Start(ctx context.Context) error                    { return nil }
func (m *mockPoolForCores) Stop() error                                        { return nil }
func (m *mockPoolForCores) Connect(ctx context.Context, node NodeConfig) error { return nil }
func (m *mockPoolForCores) Disconnect(nodeID int64) error                      { return nil }
func (m *mockPoolForCores) Reconnect(nodeID int64) error                       { return nil }
func (m *mockPoolForCores) All() []*NodeConnection                             { return nil }
func (m *mockPoolForCores) OnStatusChange(fn StatusChangeFunc)                 {}
func (m *mockPoolForCores) Get(nodeID int64) (*NodeConnection, error) {
	n, ok := m.nodes[nodeID]
	if !ok {
		return nil, fmt.Errorf("node %d not found", nodeID)
	}
	return n, nil
}
func (m *mockPoolForCores) Status(nodeID int64) NodeStatus {
	n, ok := m.nodes[nodeID]
	if !ok {
		return StatusOffline
	}
	return n.Status
}
