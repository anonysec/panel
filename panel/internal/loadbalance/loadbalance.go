//go:build !lite

package loadbalance

import "errors"

// NodeLoad represents the current load state of a node.
type NodeLoad struct {
	NodeID         int64
	ActiveSessions int
	MaxCapacity    int
}

// CalculateLoad returns the load percentage for a node.
// If capacity <= 0, returns 100.0 (full).
func CalculateLoad(active int, capacity int) float64 {
	if capacity <= 0 {
		return 100.0
	}
	return (float64(active) / float64(capacity)) * 100.0
}

// SelectNode picks the node with the lowest load percentage below
// the overload threshold. Returns the NodeID of the selected node.
// If no nodes are below threshold, returns error "all nodes overloaded".
// If nodes slice is empty, returns error "no nodes available".
func SelectNode(nodes []NodeLoad, overloadThreshold float64) (int64, error) {
	if len(nodes) == 0 {
		return 0, errors.New("no nodes available")
	}

	bestID := int64(0)
	bestLoad := float64(-1)
	found := false

	for _, n := range nodes {
		load := CalculateLoad(n.ActiveSessions, n.MaxCapacity)
		if load >= overloadThreshold {
			continue
		}
		if !found || load < bestLoad {
			bestID = n.NodeID
			bestLoad = load
			found = true
		}
	}

	if !found {
		return 0, errors.New("all nodes overloaded")
	}
	return bestID, nil
}

// IsOverloaded returns true if the node's calculated load percentage
// is at or above the given threshold.
func IsOverloaded(node NodeLoad, threshold float64) bool {
	return CalculateLoad(node.ActiveSessions, node.MaxCapacity) >= threshold
}
