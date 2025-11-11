package kademliadfs

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

type Node struct {
	Self         Contact
	RoutingTable *RoutingTable
	Storage      map[NodeId]byte

	mu sync.RWMutex
}

type NodeId [idLength]byte

func NewNodeId(name string) NodeId {
	return sha256.Sum256([]byte(name))
}

func (nodeId NodeId) String() string {
	return hex.EncodeToString(nodeId[:])
}
