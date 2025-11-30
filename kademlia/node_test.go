package kademliadfs_test

import (
	"net"
	"testing"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
)

func TestNode_Lookup(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for receiver constructor.
		id      kademliadfs.NodeId
		ip      net.IP
		port    int
		network kademliadfs.Network
		// Named input parameters for target function.
		targetID kademliadfs.NodeId
		want     []kademliadfs.Contact
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := kademliadfs.NewNode(tt.id, tt.ip, tt.port, tt.network)
			got := node.Lookup(tt.targetID)
			// TODO: update the condition below to compare got with tt.want.
			if true {
				t.Errorf("Lookup() = %v, want %v", got, tt.want)
			}
		})
	}
}
