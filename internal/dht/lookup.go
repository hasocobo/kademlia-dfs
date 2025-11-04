package dht

import (
	"sync"
	"time"
)

// LookupResult represents the result of a lookup operation
type LookupResult struct {
	Nodes []*Node
	Value []byte // Optional value found during lookup
}

// Lookup performs a Kademlia lookup for a target ID
func (dht *DHT) Lookup(target NodeID, findValue bool) (*LookupResult, error) {
	// Get initial candidates from routing table
	closest := dht.routingTable.FindClosestNodes(target, Alpha)
	
	if len(closest) == 0 {
		return &LookupResult{Nodes: []*Node{}}, nil
	}

	// Track nodes we've queried
	queried := make(map[NodeID]bool)
	results := make(map[NodeID]*Node)
	
	// Initialize with closest nodes
	for _, node := range closest {
		results[node.ID] = node
	}

	// Iterative lookup
	prevClosest := []*Node{}
	iterations := 0
	maxIterations := 20

	for iterations < maxIterations {
		// Find alpha closest nodes we haven't queried yet
		toQuery := []*Node{}
		sorted := sortByDistance(results, target)
		
		for _, node := range sorted {
			if !queried[node.ID] && len(toQuery) < Alpha {
				toQuery = append(toQuery, node)
			}
		}

		if len(toQuery) == 0 {
			break
		}

		// Query nodes in parallel
		var wg sync.WaitGroup
		var mu sync.Mutex
		newNodes := make(map[NodeID]*Node)
		var foundValue []byte
		var foundValueNode *Node

		for _, node := range toQuery {
			wg.Add(1)
			go func(n *Node) {
				defer wg.Done()
				queried[n.ID] = true

				// Perform RPC call
				if findValue {
					// FindValue RPC
					nodes, value, err := dht.findValueRPC(n, target)
					if err == nil && value != nil {
						mu.Lock()
						foundValue = value
						foundValueNode = n
						mu.Unlock()
						return // Found value
					}
					if err == nil {
						for _, newNode := range nodes {
							mu.Lock()
							newNodes[newNode.ID] = newNode
							mu.Unlock()
						}
					}
				} else {
					// FindNode RPC
					nodes, err := dht.findNodeRPC(n, target)
					if err == nil {
						for _, newNode := range nodes {
							mu.Lock()
							newNodes[newNode.ID] = newNode
							mu.Unlock()
						}
					}
				}
			}(node)
		}

		wg.Wait()

		// If value found, return early
		if foundValue != nil {
			return &LookupResult{
				Nodes: []*Node{foundValueNode},
				Value: foundValue,
			}, nil
		}

		// Merge new nodes into results
		for id, node := range newNodes {
			if _, exists := results[id]; !exists {
				results[id] = node
			}
		}

		// Get k closest nodes
		currentClosest := sortByDistance(results, target)
		if len(currentClosest) > K {
			currentClosest = currentClosest[:K]
		}

		// Check if we've converged
		if hasConverged(prevClosest, currentClosest) {
			break
		}

		prevClosest = currentClosest
		iterations++
	}

	// Return k closest nodes
	finalClosest := sortByDistance(results, target)
	if len(finalClosest) > K {
		finalClosest = finalClosest[:K]
	}

	return &LookupResult{Nodes: finalClosest}, nil
}

// sortByDistance sorts nodes by distance to target
func sortByDistance(nodes map[NodeID]*Node, target NodeID) []*Node {
	result := make([]*Node, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, node)
	}

	// Simple selection sort
	for i := 0; i < len(result); i++ {
		closestIdx := i
		for j := i + 1; j < len(result); j++ {
			if result[j].ID.Distance(target) < result[closestIdx].ID.Distance(target) {
				closestIdx = j
			}
		}
		result[i], result[closestIdx] = result[closestIdx], result[i]
	}

	return result
}

// hasConverged checks if the lookup has converged
func hasConverged(prev, curr []*Node) bool {
	if len(prev) != len(curr) {
		return false
	}
	
	prevSet := make(map[NodeID]bool)
	for _, node := range prev {
		prevSet[node.ID] = true
	}

	for _, node := range curr {
		if !prevSet[node.ID] {
			return false
		}
	}
	return true
}

// findNodeRPC performs a FindNode RPC call using the network layer
func (dht *DHT) findNodeRPC(node *Node, target NodeID) ([]*Node, error) {
	if dht.network == nil {
		return []*Node{}, nil
	}
	return dht.network.SendFindNode(node, target)
}

// findValueRPC performs a FindValue RPC call using the network layer
func (dht *DHT) findValueRPC(node *Node, target NodeID) ([]*Node, []byte, error) {
	if dht.network == nil {
		return []*Node{}, nil, nil
	}
	return dht.network.SendFindValue(node, target)
}

