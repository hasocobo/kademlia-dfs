package kademliadfs

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"sync"
)

type Node struct {
	Self         Contact
	RoutingTable *RoutingTable
	Storage      map[NodeId][]byte

	mu sync.RWMutex
}

func NewNode(id NodeId, ip net.IP, port int) *Node {
	contact := Contact{ID: id, IP: ip, Port: port}
	rt := NewRoutingTable(id, func(c Contact) bool { return true })
	storage := make(map[NodeId][]byte)

	return &Node{Self: contact, RoutingTable: rt, Storage: storage}
}

type NodeId [idLength]byte

func NewNodeId(name string) NodeId {
	return sha256.Sum256([]byte(name))
}

func (nodeId NodeId) String() string {
	return hex.EncodeToString(nodeId[:])
}

func (node *Node) HandlePing(requester Contact) bool {
	node.RoutingTable.Update(requester)

	return true
}

// This is the handler. Remote Producer Call(RPC) will call this function with its own Contact information.
// e.g with a 3 parameter function:
// FindNode(requester: Contact, recipientNodeToAsk: Node, targetId: NodeId) and in turn this
// will be handled by the function below by the recipientNodeToAsk.
func (node *Node) HandleFindNode(requester Contact, targetID NodeId) []Contact {
	node.RoutingTable.Update(requester)

	return node.RoutingTable.FindClosest(targetID, k)
}

// Return the value if it exists, return closest nodes if not
func (node *Node) HandleFindValue(requester Contact, key NodeId) ([]byte, []Contact) {
	node.RoutingTable.Update(requester)

	node.mu.RLock()
	defer node.mu.RUnlock()
	if value, ok := node.Storage[key]; ok {
		return value, nil
	}

	closestContacts := node.RoutingTable.FindClosest(key, k)
	return nil, closestContacts
}

func (node *Node) HandleStore(requester Contact, key NodeId, value []byte) {
	node.RoutingTable.Update(requester)

	node.mu.Lock()
	defer node.mu.RUnlock()

	node.Storage[key] = value
}
