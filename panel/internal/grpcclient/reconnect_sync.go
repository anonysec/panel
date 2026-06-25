package grpcclient

import (
	"context"
	"log"
)

// RegisterReconnectSync registers an OnStatusChange callback on the pool that
// triggers a full user sync whenever a node transitions from "offline" to "online".
// This ensures that nodes returning from downtime receive the latest user data
// for all cores they serve.
func RegisterReconnectSync(pool Pool, syncService *UserSyncService) {
	pool.OnStatusChange(func(nodeID int64, old, new NodeStatus) {
		if old == StatusOffline && new == StatusOnline {
			log.Printf("[knode] Node %d reconnected (offline → online), triggering full user sync", nodeID)
			go func() {
				ctx := context.Background()
				if err := syncService.FullSyncForNode(ctx, nodeID); err != nil {
					log.Printf("[knode] Full sync failed for reconnected node %d: %v", nodeID, err)
				}
			}()
		}
	})
}
