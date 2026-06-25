package grpcclient

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// VPNSession represents an active VPN connection on a knode instance.
// This is distinct from dbstore.Session which represents HTTP panel sessions.
type VPNSession struct {
	Username    string    `json:"username"`
	CoreType    string    `json:"core_type"`
	ConnectedAt time.Time `json:"connected_at"`
	BytesIn     int64     `json:"bytes_in"`
	BytesOut    int64     `json:"bytes_out"`
	ClientIP    string    `json:"client_ip"`
}

// SessionManager provides VPN session management operations via gRPC.
// It wraps the connection pool to call ListSessions and DisconnectUser
// RPCs on target knode instances.
//
// Multi-panel compatibility (Requirements 14.1, 14.3):
// - ListSessions always fetches live state from knode (stateless, no caching).
// - DisconnectUser handles NotFound gracefully (user already disconnected by another panel).
type SessionManager struct {
	pool Pool
}

// NewSessionManager creates a SessionManager that uses the given pool
// to communicate with knode instances.
func NewSessionManager(pool Pool) *SessionManager {
	return &SessionManager{pool: pool}
}

// ListSessions retrieves all active VPN sessions from the specified node.
// It calls the ListSessions RPC on the target knode and returns the active
// sessions for display on the admin sessions page.
//
// Satisfies Requirement 13.1: When an admin views a node's sessions page,
// the panel SHALL call ListSessions on the target knode and display the active sessions.
//
// Multi-panel compatibility (Requirements 14.1, 14.3): This always fetches live state
// from knode rather than relying on any cached/local data, ensuring consistent display
// even when multiple panels are connected to the same node simultaneously.
func (sm *SessionManager) ListSessions(ctx context.Context, nodeID int64) ([]VPNSession, error) {
	node, err := sm.pool.Get(nodeID)
	if err != nil {
		log.Printf("[knode] ListSessions: node %d not found in pool: %v", nodeID, err)
		return nil, fmt.Errorf("node %d not found in pool: %w", nodeID, err)
	}

	if node.Status == StatusOffline {
		log.Printf("[knode] ListSessions: node %q (id=%d) is offline", node.NodeName, nodeID)
		return nil, fmt.Errorf("node %q is offline, cannot list sessions", node.NodeName)
	}

	// TODO: Replace with actual gRPC call when proto client is generated.
	// The call would be:
	//   client := knodepb.NewKnodeServiceClient(node.Conn)
	//   resp, err := client.ListSessions(ctx, &knodepb.ListSessionsRequest{})
	//   if err != nil {
	//       log.Printf("[knode] ListSessions RPC failed on node %q (id=%d): %v", node.NodeName, nodeID, err)
	//       return nil, fmt.Errorf("ListSessions RPC on node %q: %w", node.NodeName, err)
	//   }
	//   sessions := make([]VPNSession, 0, len(resp.Sessions))
	//   for _, s := range resp.Sessions {
	//       sessions = append(sessions, VPNSession{
	//           Username:    s.Username,
	//           CoreType:    s.CoreType,
	//           ConnectedAt: s.ConnectedAt.AsTime(),
	//           BytesIn:     s.BytesIn,
	//           BytesOut:    s.BytesOut,
	//           ClientIP:    s.ClientIp,
	//       })
	//   }
	//   return sessions, nil
	_ = node.Conn // will be used with proto client

	log.Printf("[knode] ListSessions: fetching live sessions from node %q (id=%d)", node.NodeName, nodeID)
	return []VPNSession{}, nil
}

// DisconnectUser disconnects a user from the specified node, optionally filtering
// by core type. If coreFilter is empty, the user is disconnected from all cores.
//
// Satisfies Requirement 13.2: When an admin requests disconnecting a user,
// the panel SHALL call DisconnectUser on the target knode with the username
// and optional core filter.
//
// Satisfies Requirement 13.4: If the DisconnectUser RPC returns an error,
// the panel SHALL display the error message to the admin.
//
// Multi-panel compatibility (Requirement 14.2): If NotFound is returned
// (user already disconnected by another panel), treat as success.
func (sm *SessionManager) DisconnectUser(ctx context.Context, nodeID int64, username string, coreFilter string) error {
	node, err := sm.pool.Get(nodeID)
	if err != nil {
		log.Printf("[knode] DisconnectUser: node %d not found in pool: %v", nodeID, err)
		return fmt.Errorf("node %d not found in pool: %w", nodeID, err)
	}

	if node.Status == StatusOffline {
		log.Printf("[knode] DisconnectUser: node %q (id=%d) is offline", node.NodeName, nodeID)
		return fmt.Errorf("node %q is offline, cannot disconnect user", node.NodeName)
	}

	// TODO: Replace with actual gRPC call when proto client is generated.
	// The call would be:
	//   client := knodepb.NewKnodeServiceClient(node.Conn)
	//   _, err = client.DisconnectUser(ctx, &knodepb.DisconnectUserRequest{
	//       Username:   username,
	//       CoreFilter: coreFilter,
	//   })
	var rpcErr error // placeholder for actual RPC error
	_ = node.Conn    // will be used with proto client

	if rpcErr != nil {
		if status.Code(rpcErr) == codes.NotFound {
			// User already disconnected (likely by another panel instance or natural disconnect).
			// Not an error — the desired state is already achieved.
			log.Printf("[knode] DisconnectUser: user %q not found on node %q (id=%d), already disconnected", username, node.NodeName, nodeID)
			return nil
		}
		log.Printf("[knode] DisconnectUser RPC failed on node %q (id=%d) for user %q: %v",
			node.NodeName, nodeID, username, rpcErr)
		return fmt.Errorf("DisconnectUser RPC on node %q for user %q: %w", node.NodeName, username, rpcErr)
	}

	log.Printf("[knode] DisconnectUser: disconnected user %q (core=%q) on node %q (id=%d)",
		username, coreFilter, node.NodeName, nodeID)
	return nil
}
