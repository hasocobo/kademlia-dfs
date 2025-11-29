package main

import (
	"fmt"
	"net"
	"strconv"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
)

func main() {
	simulationNetwork := kademliadfs.NewSimNetwork()

	testNode1 := kademliadfs.NewNode(
		kademliadfs.NewNodeId("test"),
		net.IPv4zero,
		10000,
		simulationNetwork)

	simulationNetwork.Register(testNode1)

	testNode2 := kademliadfs.NewNode(
		kademliadfs.NewNodeId("testt"),
		net.IPv4zero,
		10000,
		simulationNetwork)

	simulationNetwork.Register(testNode2)
	testNode2.Join(testNode1.Self)

	testNode3 := kademliadfs.NewNode(
		kademliadfs.NewNodeId("testtt"),
		net.IPv4zero,
		10000,
		simulationNetwork)

	simulationNetwork.Register(testNode3)
	testNode3.Join(testNode1.Self)

	for i := range 10 {
		newNode := kademliadfs.NewNode(
			kademliadfs.NewNodeId("test "+strconv.Itoa(i+1)),
			net.IPv4zero,
			10000+i,
			simulationNetwork)
		simulationNetwork.Register(newNode)
		newNode.Join(testNode1.Self)
	}

	lookupResult := testNode1.Lookup(kademliadfs.NewNodeId("test 233"))

	for _, v := range lookupResult {
		fmt.Printf("%v, %v", v, kademliadfs.XorDistance(v.ID, kademliadfs.NewNodeId("test 233")).String())
		fmt.Println()
	}
}
