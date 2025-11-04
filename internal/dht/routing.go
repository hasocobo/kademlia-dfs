package dht

import (
	"sync"
)

// RoutingTable manages the k-buckets for a node
type RoutingTable struct {
	mu       sync.RWMutex
	nodeID   NodeID
	buckets  [160]*KBucket
	selfNode *Node
}

// NewRoutingTable creates a new routing table for a node
func NewRoutingTable(nodeID NodeID, selfNode *Node) *RoutingTable {
	rt := &RoutingTable{
		nodeID:   nodeID,
		selfNode: selfNode,
	}

	// Initialize all buckets
	for i := 0; i < 160; i++ {
		rt.buckets[i] = NewKBucket(i)
	}

	return rt
}

// AddNode adds a node to the appropriate bucket
func (rt *RoutingTable) AddNode(node *Node) {
	if node.ID == rt.nodeID {
		return // Don't add self
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	bucketIdx := rt.getBucketIndex(node.ID)
	rt.buckets[bucketIdx].Add(node)
}

// RemoveNode removes a node from the routing table
func (rt *RoutingTable) RemoveNode(nodeID NodeID) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	bucketIdx := rt.getBucketIndex(nodeID)
	rt.buckets[bucketIdx].Remove(nodeID)
}

// FindClosestNodes finds the k closest nodes to the target ID
func (rt *RoutingTable) FindClosestNodes(target NodeID, k int) []*Node {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	var candidates []*Node
	bucketIdx := rt.getBucketIndex(target)

	// Start from the target's bucket and expand outward
	for offset := 0; offset < 160 && len(candidates) < k*2; offset++ {
		// Check bucket before
		if bucketIdx-offset >= 0 {
			candidates = append(candidates, rt.buckets[bucketIdx-offset].GetNodes()...)
		}
		// Check bucket after
		if bucketIdx+offset < 160 && offset > 0 {
			candidates = append(candidates, rt.buckets[bucketIdx+offset].GetNodes()...)
		}
	}

	// Remove duplicates
	seen := make(map[NodeID]bool)
	unique := []*Node{}
	for _, node := range candidates {
		if !seen[node.ID] && node.ID != rt.nodeID {
			seen[node.ID] = true
			unique = append(unique, node)
		}
	}

	// Sort by distance and return k closest
	if len(unique) <= k {
		return unique
	}

	// Simple selection sort
	for i := 0; i < k; i++ {
		closestIdx := i
		for j := i + 1; j < len(unique); j++ {
			if unique[j].ID.Distance(target) < unique[closestIdx].ID.Distance(target) {
				closestIdx = j
			}
		}
		unique[i], unique[closestIdx] = unique[closestIdx], unique[i]
	}

	return unique[:k]
}

// getBucketIndex returns the bucket index for a given node ID
func (rt *RoutingTable) getBucketIndex(nodeID NodeID) int {
	xor := rt.nodeID.XOR(nodeID)
	
	// Find the first non-zero bit
	for i := 0; i < 20; i++ {
		if xor[i] != 0 {
			for j := 0; j < 8; j++ {
				if (xor[i]>>(7-j))&1 != 0 {
					return i*8 + j
				}
			}
		}
	}
	return 159 // Same node (shouldn't happen)
}

// GetNodeCount returns the total number of nodes in the routing table
func (rt *RoutingTable) GetNodeCount() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	count := 0
	for i := 0; i < 160; i++ {
		count += rt.buckets[i].Size()
	}
	return count
}

