package dht

import (
	"sync"
	"time"
)

const (
	// K is the bucket size parameter (typically 20)
	K = 20
	// Alpha is the concurrency parameter for lookups
	Alpha = 3
)

// KBucket represents a k-bucket in the Kademlia routing table
type KBucket struct {
	mu       sync.RWMutex
	nodes    []*Node
	lastSeen time.Time
	prefix   int // prefix length for this bucket
}

// NewKBucket creates a new k-bucket
func NewKBucket(prefix int) *KBucket {
	return &KBucket{
		nodes:    make([]*Node, 0, K),
		lastSeen: time.Now(),
		prefix:   prefix,
	}
}

// Add adds a node to the bucket if there's space or replaces least recently seen
func (kb *KBucket) Add(node *Node) bool {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	// Update last seen if node already exists
	for i, n := range kb.nodes {
		if n.ID == node.ID {
			kb.nodes[i].LastSeen = time.Now()
			kb.lastSeen = time.Now()
			return true
		}
	}

	// Add if space available
	if len(kb.nodes) < K {
		node.LastSeen = time.Now()
		kb.nodes = append(kb.nodes, node)
		kb.lastSeen = time.Now()
		return true
	}

	// Replace least recently seen if all nodes are responsive
	// Otherwise, don't add (simplified - in real implementation, ping least recently seen)
	// For now, we'll replace the oldest node
	oldestIdx := 0
	for i := 1; i < len(kb.nodes); i++ {
		if kb.nodes[i].LastSeen.Before(kb.nodes[oldestIdx].LastSeen) {
			oldestIdx = i
		}
	}

	kb.nodes[oldestIdx] = node
	node.LastSeen = time.Now()
	kb.lastSeen = time.Now()
	return true
}

// Remove removes a node from the bucket
func (kb *KBucket) Remove(nodeID NodeID) bool {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	for i, n := range kb.nodes {
		if n.ID == nodeID {
			kb.nodes = append(kb.nodes[:i], kb.nodes[i+1:]...)
			return true
		}
	}
	return false
}

// GetNodes returns a copy of nodes in the bucket
func (kb *KBucket) GetNodes() []*Node {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	result := make([]*Node, len(kb.nodes))
	copy(result, kb.nodes)
	return result
}

// Size returns the number of nodes in the bucket
func (kb *KBucket) Size() int {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	return len(kb.nodes)
}

// GetClosestNodes returns the k closest nodes to the target ID
func (kb *KBucket) GetClosestNodes(target NodeID, k int) []*Node {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	if len(kb.nodes) == 0 {
		return []*Node{}
	}

	// Sort nodes by distance to target
	nodes := make([]*Node, len(kb.nodes))
	copy(nodes, kb.nodes)

	// Simple selection sort (could be optimized)
	for i := 0; i < len(nodes) && i < k; i++ {
		closestIdx := i
		for j := i + 1; j < len(nodes); j++ {
			if nodes[j].ID.Distance(target) < nodes[closestIdx].ID.Distance(target) {
				closestIdx = j
			}
		}
		nodes[i], nodes[closestIdx] = nodes[closestIdx], nodes[i]
	}

	if len(nodes) > k {
		return nodes[:k]
	}
	return nodes
}

