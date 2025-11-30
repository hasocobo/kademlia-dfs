package kademliadfs

import (
	"net"
	"strconv"
	"testing"
)

func TestNodeLookup_EmptyRoutingTable_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	simulationNetwork := NewSimNetwork()
	selfID := NewNodeId("test-1")
	targetID := NewNodeId("test-2")
	node := NewNode(selfID, net.IPv4zero, 10000, simulationNetwork)

	lookupResult := node.Lookup(targetID)

	if len(lookupResult) != 0 {
		t.Fatalf("Unexpected length. Want: %v, Got: %v", 0, len(lookupResult))
	}
}

func TestNodeJoinAndLookup(t *testing.T) {
	t.Parallel()

	simulationNetwork := NewSimNetwork()

	bootstrapNode := NewNode(
		NewNodeId("bootstrap-node"),
		net.IPv4zero,
		10000,
		simulationNetwork)
	simulationNetwork.Register(bootstrapNode)

	nodeCount := 100
	nodes := make([]*Node, nodeCount)
	for i := range nodeCount {
		node := NewNode(
			NewNodeId("test-node-"+strconv.Itoa(i)),
			net.IPv4zero,
			10001+i,
			simulationNetwork)
		simulationNetwork.Register(node)
		node.Join(bootstrapNode.Self)
		nodes[i] = node
	}

	contacts := bootstrapNode.RoutingTable.FindClosest(bootstrapNode.Self.ID, k)
	if len(contacts) == 0 {
		t.Fatal("Unexpected length 0, bootstrap node can't have 0 contacts after other nodes join")
	}

	// Pick a random number and do a lookup from another node, the target node must be in the lookupResult
	targetNode := nodes[50]
	searchingNode := nodes[10]

	lookupResult := searchingNode.Lookup(targetNode.Self.ID)

	if len(lookupResult) == 0 {
		t.Fatal("Unexpected length 0")
	}

	// Verify the target node is findable (should be in top results)
	found := false
	for _, contact := range lookupResult {
		if contact.ID == targetNode.Self.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Target node should be discoverable with lookup")
	}
}
