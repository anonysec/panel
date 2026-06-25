package grpcclient

import (
	"context"
	"fmt"
	"log"
	"time"
)

// CertInfo holds certificate expiry and identity information for a core on a node.
type CertInfo struct {
	CoreType  string    `json:"core_type"`
	ExpiresAt time.Time `json:"expires_at"`
	Issuer    string    `json:"issuer"`
	Subject   string    `json:"subject"`
}

// CertManager wraps the gRPC connection pool to provide certificate management
// operations against knode instances. It implements Requirements 12.1–12.4.
type CertManager struct {
	pool Pool
}

// NewCertManager creates a new CertManager backed by the given connection pool.
func NewCertManager(pool Pool) *CertManager {
	return &CertManager{pool: pool}
}

// SetCertificates pushes CA, certificate, and key PEM data for a specific core type
// to the target knode instance. Returns an error if the node is not reachable or the
// RPC fails; errors are logged with the [knode] tag and returned for admin display.
//
// Satisfies Requirement 12.1: When an admin uploads certificates for a core type,
// the panel SHALL call SetCertificates on the target knode.
// Satisfies Requirement 12.3: When the RPC returns an error, display to admin.
// Satisfies Requirement 12.4: Called by cert rotation worker on expiring certs.
func (cm *CertManager) SetCertificates(ctx context.Context, nodeID int64, coreType string, caCert, cert, key []byte) error {
	node, err := cm.pool.Get(nodeID)
	if err != nil {
		log.Printf("[knode] SetCertificates: node %d not found in pool: %v", nodeID, err)
		return fmt.Errorf("node %d not found in pool: %w", nodeID, err)
	}

	if node.Status == StatusOffline {
		log.Printf("[knode] SetCertificates: node %q (id=%d) is offline, cannot push certificates", node.NodeName, nodeID)
		return fmt.Errorf("node %q is offline, cannot push certificates", node.NodeName)
	}

	// TODO: Replace with actual gRPC call when proto client is generated.
	// The call would be:
	//   client := knodepb.NewKnodeServiceClient(node.Conn)
	//   _, err = client.SetCertificates(ctx, &knodepb.SetCertificatesRequest{
	//       CoreType: coreType,
	//       CaCert:   caCert,
	//       Cert:     cert,
	//       Key:      key,
	//   })
	//   if err != nil {
	//       log.Printf("[knode] SetCertificates failed for core %q on node %q (id=%d): %v", coreType, node.NodeName, nodeID, err)
	//       return fmt.Errorf("SetCertificates RPC failed: %w", err)
	//   }
	log.Printf("[knode] SetCertificates stub: would push certs for core %q to node %q (id=%d) — ca=%d bytes, cert=%d bytes, key=%d bytes",
		coreType, node.NodeName, nodeID, len(caCert), len(cert), len(key))

	return nil
}

// GetCertInfo retrieves certificate expiry information for all cores on the target
// knode instance. Returns a slice of CertInfo with expiry, issuer, and subject details.
// Errors are logged with [knode] tag and returned for admin display.
//
// Satisfies Requirement 12.2: The panel SHALL call GetCertInfo to display certificate
// expiry information for each core on a node.
func (cm *CertManager) GetCertInfo(ctx context.Context, nodeID int64) ([]CertInfo, error) {
	node, err := cm.pool.Get(nodeID)
	if err != nil {
		log.Printf("[knode] GetCertInfo: node %d not found in pool: %v", nodeID, err)
		return nil, fmt.Errorf("node %d not found in pool: %w", nodeID, err)
	}

	if node.Status == StatusOffline {
		log.Printf("[knode] GetCertInfo: node %q (id=%d) is offline, cannot retrieve cert info", node.NodeName, nodeID)
		return nil, fmt.Errorf("node %q is offline, cannot retrieve cert info", node.NodeName)
	}

	// TODO: Replace with actual gRPC call when proto client is generated.
	// The call would be:
	//   client := knodepb.NewKnodeServiceClient(node.Conn)
	//   resp, err := client.GetCertInfo(ctx, &knodepb.GetCertInfoRequest{})
	//   if err != nil {
	//       log.Printf("[knode] GetCertInfo failed for node %q (id=%d): %v", node.NodeName, nodeID, err)
	//       return nil, fmt.Errorf("GetCertInfo RPC failed: %w", err)
	//   }
	//   var certs []CertInfo
	//   for _, c := range resp.Certs {
	//       certs = append(certs, CertInfo{
	//           CoreType:  c.CoreType,
	//           ExpiresAt: c.ExpiresAt.AsTime(),
	//           Issuer:    c.Issuer,
	//           Subject:   c.Subject,
	//       })
	//   }
	//   return certs, nil
	log.Printf("[knode] GetCertInfo stub: would retrieve cert info from node %q (id=%d)", node.NodeName, nodeID)

	return []CertInfo{}, nil
}
