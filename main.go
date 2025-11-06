package main

import (
	"crypto/sha1"
	"fmt"
)

type NodeId [20]byte

func NewNodeId(name string) NodeId {
	return sha1.Sum([]byte(name)) // Returns 20 byte hash
}

func (nodeId NodeId) String() string {
	return string(nodeId[:]) // ":" is needed for arrays, it means from beginning to end
}

func main() {
	node := NewNodeId("test")
	fmt.Printf("%s", node.String())
}
