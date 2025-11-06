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
	return string(nodeId[:]) // ":" is needed for arrays, it means "from beginning to end"
}

func xorDistance(a, b NodeId) int {
	dist := 0
	for i := 0; i < len(a); i++ {
		dist += int(a[i] ^ b[i])
	}

	return dist
}

func main() {
	n1 := NewNodeId("testa")
	n2 := NewNodeId("testb")
	fmt.Println(n1.String())
	fmt.Println(n2.String())
	fmt.Println(xorDistance(n1, n2))
}
