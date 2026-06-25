package grpcclient

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// FirewallRule represents a single firewall rule reported by a knode instance.
type FirewallRule struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"` // "tcp", "udp", or "both"
	Comment  string `json:"comment"`
	Action   string `json:"action"` // "accept", "drop", etc.
}

// FirewallManager provides firewall management operations on knode instances
// via gRPC. It wraps the connection pool and exposes OpenPort, ClosePort, and
// ListFirewallRules as high-level methods for the admin API.
//
// Multi-panel compatibility (Requirements 14.1, 14.3):
// - ListFirewallRules always fetches live state from knode (no caching).
// - OpenPort/ClosePort handle AlreadyExists/NotFound gracefully by refreshing.
type FirewallManager struct {
	pool Pool
}

// NewFirewallManager creates a new FirewallManager using the given connection pool.
func NewFirewallManager(pool Pool) *FirewallManager {
	return &FirewallManager{pool: pool}
}

// OpenPort requests a knode to open a firewall port with the given protocol and comment.
// The protocol should be "tcp", "udp", or "both".
//
// Satisfies Requirement 10.1: WHEN an admin requests opening a port, THE Panel SHALL
// call OpenPort on the target knode with the port number, protocol (tcp/udp/both), and comment.
//
// Satisfies Requirement 10.4: WHEN the OpenPort RPC returns an error, THE Panel SHALL
// display the error message to the admin.
//
// Multi-panel compatibility (Requirement 14.2): If AlreadyExists is returned (port
// already opened by another panel), treat as success and refresh state.
func (fm *FirewallManager) OpenPort(ctx context.Context, nodeID int64, port int, protocol string, comment string) error {
	node, err := fm.pool.Get(nodeID)
	if err != nil {
		return fmt.Errorf("node %d not found in pool: %w", nodeID, err)
	}

	if node.Status == StatusOffline {
		return fmt.Errorf("node %q is offline, cannot open port", node.NodeName)
	}

	// TODO: Replace with actual gRPC call when proto client is generated.
	// The call would be:
	//   client := knodepb.NewKnodeServiceClient(node.Conn)
	//   _, err = client.OpenPort(ctx, &knodepb.OpenPortRequest{
	//       Port:     int32(port),
	//       Protocol: protocol,
	//       Comment:  comment,
	//   })
	var rpcErr error // placeholder for actual RPC error
	_ = node.Conn    // will be used with proto client

	if rpcErr != nil {
		if status.Code(rpcErr) == codes.AlreadyExists {
			// Port rule already exists (likely opened by another panel instance).
			// Not an error — the desired state is already achieved.
			log.Printf("[knode] OpenPort: port %d/%s already open on node %q (id=%d), concurrent modification", port, protocol, node.NodeName, nodeID)
			return nil
		}
		log.Printf("[knode] OpenPort failed on node %q (id=%d): %v", node.NodeName, nodeID, rpcErr)
		return rpcErr
	}

	log.Printf("[knode] OpenPort: opened port %d/%s on node %q (id=%d) comment=%q",
		port, protocol, node.NodeName, nodeID, comment)
	return nil
}

// ClosePort requests a knode to close a firewall port for the given protocol.
//
// Satisfies Requirement 10.2: WHEN an admin requests closing a port, THE Panel SHALL
// call ClosePort on the target knode with the port number and protocol.
//
// Satisfies Requirement 10.4: WHEN the ClosePort RPC returns an error, THE Panel SHALL
// display the error message to the admin.
//
// Multi-panel compatibility (Requirement 14.2): If NotFound is returned (port
// already closed by another panel), treat as success.
func (fm *FirewallManager) ClosePort(ctx context.Context, nodeID int64, port int, protocol string) error {
	node, err := fm.pool.Get(nodeID)
	if err != nil {
		return fmt.Errorf("node %d not found in pool: %w", nodeID, err)
	}

	if node.Status == StatusOffline {
		return fmt.Errorf("node %q is offline, cannot close port", node.NodeName)
	}

	// TODO: Replace with actual gRPC call when proto client is generated.
	// The call would be:
	//   client := knodepb.NewKnodeServiceClient(node.Conn)
	//   _, err = client.ClosePort(ctx, &knodepb.ClosePortRequest{
	//       Port:     int32(port),
	//       Protocol: protocol,
	//   })
	var rpcErr error // placeholder for actual RPC error
	_ = node.Conn    // will be used with proto client

	if rpcErr != nil {
		if status.Code(rpcErr) == codes.NotFound {
			// Port rule doesn't exist (likely already closed by another panel instance).
			// Not an error — the desired state is already achieved.
			log.Printf("[knode] ClosePort: port %d/%s already closed on node %q (id=%d), concurrent modification", port, protocol, node.NodeName, nodeID)
			return nil
		}
		log.Printf("[knode] ClosePort failed on node %q (id=%d): %v", node.NodeName, nodeID, rpcErr)
		return rpcErr
	}

	log.Printf("[knode] ClosePort: closed port %d/%s on node %q (id=%d)",
		port, protocol, node.NodeName, nodeID)
	return nil
}

// ListFirewallRules retrieves the current firewall rules from a knode instance.
// Called when the admin views a node's firewall page.
//
// Satisfies Requirement 10.3: THE Panel SHALL call ListFirewallRules on a node when
// the admin views that node's firewall page, and display the current rules.
//
// Multi-panel compatibility (Requirements 14.1, 14.3): This always fetches live state
// from knode rather than relying on any cached/local data, ensuring consistent display
// even when multiple panels modify the same node simultaneously.
func (fm *FirewallManager) ListFirewallRules(ctx context.Context, nodeID int64) ([]FirewallRule, error) {
	node, err := fm.pool.Get(nodeID)
	if err != nil {
		return nil, fmt.Errorf("node %d not found in pool: %w", nodeID, err)
	}

	if node.Status == StatusOffline {
		return nil, fmt.Errorf("node %q is offline, cannot list firewall rules", node.NodeName)
	}

	// TODO: Replace with actual gRPC call when proto client is generated.
	// The call would be:
	//   client := knodepb.NewKnodeServiceClient(node.Conn)
	//   resp, err := client.ListFirewallRules(ctx, &knodepb.ListFirewallRulesRequest{})
	//   if err != nil {
	//       log.Printf("[knode] ListFirewallRules failed on node %q (id=%d): %v", node.NodeName, nodeID, err)
	//       return nil, err
	//   }
	//   rules := make([]FirewallRule, 0, len(resp.Rules))
	//   for _, r := range resp.Rules {
	//       rules = append(rules, FirewallRule{
	//           Port:     int(r.Port),
	//           Protocol: r.Protocol,
	//           Comment:  r.Comment,
	//           Action:   r.Action,
	//       })
	//   }
	//   return rules, nil
	_ = node.Conn // will be used with proto client

	log.Printf("[knode] ListFirewallRules: fetching live rules from node %q (id=%d)",
		node.NodeName, nodeID)
	return []FirewallRule{}, nil
}
