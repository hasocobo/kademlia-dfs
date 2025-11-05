package dht

import (
	"errors"
	"math/rand"
)

// DHT represents the main Kademlia DHT structure
type DHT struct {
	nodeID       NodeID
	selfNode     *Node
	routingTable *RoutingTable
	storage      Storage
	network      Network
}

// Storage interface for storing provider records
type Storage interface {
	Store(key NodeID, value []byte) error
	Retrieve(key NodeID) ([]byte, error)
	Remove(key NodeID) error
}

// Network interface for peer communication
type Network interface {
	Start(port int) error
	Stop() error
	SendFindNode(node *Node, target NodeID) ([]*Node, error)
	SendFindValue(node *Node, target NodeID) ([]*Node, []byte, error)
	SendStore(node *Node, key NodeID, value []byte) error
	SendPing(node *Node) error
}

// NewDHT creates a new DHT instance
func NewDHT(nodeID NodeID, ip string, port int) *DHT {
	selfNode := NewNode(nodeID, ip, port)
	selfNode.IsLocal = true

	return &DHT{
		nodeID:       nodeID,
		selfNode:     selfNode,
		routingTable: NewRoutingTable(nodeID, selfNode),
	}
}

// SetStorage sets the storage backend
func (dht *DHT) SetStorage(storage Storage) {
	dht.storage = storage
}

// SetNetwork sets the network layer
func (dht *DHT) SetNetwork(network Network) {
	dht.network = network
}

// Bootstrap connects to initial nodes to populate the routing table
func (dht *DHT) Bootstrap(bootstrapNodes []*Node) error {
	for _, node := range bootstrapNodes {
		if node.ID == dht.nodeID {
			continue
		}

		// Add to routing table
		dht.routingTable.AddNode(node)

		// Perform lookup for our own ID to populate routing table
		result, err := dht.Lookup(dht.nodeID, false)
		if err != nil {
			continue
		}

		// Add discovered nodes to routing table
		for _, discoveredNode := range result.Nodes {
			dht.routingTable.AddNode(discoveredNode)
		}
	}

	return nil
}

// Store stores a key-value pair in the DHT
func (dht *DHT) Store(key NodeID, value []byte) error {
	// Find k closest nodes
	result, err := dht.Lookup(key, false)
	if err != nil {
		return err
	}

	// Store on k closest nodes
	for _, node := range result.Nodes {
		if dht.network != nil {
			dht.network.SendStore(node, key, value)
		} else if node.ID == dht.nodeID {
			// Store locally if it's us
			if dht.storage != nil {
				dht.storage.Store(key, value)
			}
		}
	}

	return nil
}

// GetValue retrieves a value from the DHT
func (dht *DHT) GetValue(key NodeID) ([]byte, error) {
	// First check local storage
	if dht.storage != nil {
		value, err := dht.storage.Retrieve(key)
		if err == nil {
			return value, nil
		}
	}

	// Perform lookup
	result, err := dht.Lookup(key, true)
	if err != nil {
		return nil, err
	}

	// If value found during lookup, return it
	if result.Value != nil {
		return result.Value, nil
	}

	return nil, ErrValueNotFound
}

// GetNodeID returns the node's ID
func (dht *DHT) GetNodeID() NodeID {
	return dht.nodeID
}

// GetSelfNode returns the self node
func (dht *DHT) GetSelfNode() *Node {
	return dht.selfNode
}

// GetRoutingTable returns the routing table
func (dht *DHT) GetRoutingTable() *RoutingTable {
	return dht.routingTable
}

// RefreshBucket refreshes a bucket by performing a lookup
func (dht *DHT) RefreshBucket(bucketIdx int) {
	// Generate a random ID in the bucket's range
	randomID := dht.generateRandomIDInBucket(bucketIdx)
	dht.Lookup(randomID, false)
}

// generateRandomIDInBucket generates a random ID in a specific bucket's range
func (dht *DHT) generateRandomIDInBucket(bucketIdx int) NodeID {
	var id NodeID
	copy(id[:], dht.nodeID[:])

	// Set prefix bits to match bucket
	byteIdx := bucketIdx / 8
	bitIdx := bucketIdx % 8

	// Flip the bit at bucketIdx
	id[byteIdx] ^= (1 << (7 - bitIdx))

	// Randomize remaining bits
	for i := byteIdx; i < 20; i++ {
		if i == byteIdx {
			// Only randomize bits after the flipped bit
			mask := byte(0xFF >> (8 - (7 - bitIdx)))
			id[i] |= mask
		} else {
			id[i] = byte(rand.Intn(256))
		}
	}

	return id
}

// Errors
var (
	ErrValueNotFound = errors.New("value not found")
)

