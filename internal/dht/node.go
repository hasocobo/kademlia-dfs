package dht

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// NodeID represents a 160-bit identifier in the Kademlia DHT
type NodeID [20]byte

// NewNodeID generates a random 160-bit node ID
func NewNodeID() NodeID {
	var id NodeID
	rand.Read(id[:])
	return id
}

// NodeIDFromString creates a NodeID from a hex string
func NodeIDFromString(s string) (NodeID, error) {
	var id NodeID
	bytes, err := hex.DecodeString(s)
	if err != nil {
		return id, err
	}
	copy(id[:], bytes)
	return id, nil
}

// String returns the hex representation of the NodeID
func (id NodeID) String() string {
	return hex.EncodeToString(id[:])
}

// XOR calculates the XOR distance between two NodeIDs
func (id NodeID) XOR(other NodeID) NodeID {
	var result NodeID
	for i := 0; i < 20; i++ {
		result[i] = id[i] ^ other[i]
	}
	return result
}

// Distance calculates the XOR distance metric (lower is closer)
func (id NodeID) Distance(other NodeID) int {
	xor := id.XOR(other)
	// Find the first non-zero byte
	for i := 0; i < 20; i++ {
		if xor[i] != 0 {
			return 20*8 - i*8 - leadingZeros(xor[i])
		}
	}
	return 0
}

// leadingZeros counts leading zeros in a byte
func leadingZeros(b byte) int {
	count := 0
	for i := 7; i >= 0; i-- {
		if (b>>i)&1 == 0 {
			count++
		} else {
			break
		}
	}
	return count
}

// Node represents a peer in the Kademlia network
type Node struct {
	ID        NodeID
	IP        string
	Port      int
	LastSeen  time.Time
	IsLocal   bool
}

// NewNode creates a new node instance
func NewNode(id NodeID, ip string, port int) *Node {
	return &Node{
		ID:       id,
		IP:       ip,
		Port:     port,
		LastSeen: time.Now(),
		IsLocal:  false,
	}
}

// Address returns the network address of the node
func (n *Node) Address() string {
	return fmt.Sprintf("%s:%d", n.IP, n.Port)
}

