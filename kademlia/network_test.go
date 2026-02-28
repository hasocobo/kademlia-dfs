package kademliadfs

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"strings"
	"testing"
)

func TestIntegration_NodesJoinAndStoreKVPAirUDP(t *testing.T) {
	clusterSize := 1000
	udpIp := net.IPv4(127, 0, 0, 1)
	bootstrapPort := 10000

	ctx := context.Background()

	bootstrapNodeNetwork, err := NewQUICNetwork(udpIp, bootstrapPort)
	if err != nil {
		t.Fatalf("error creating bootstrap node: %v", err)
	}
	bootstrapNode := NewNode(ctx, NodeId{}, udpIp, bootstrapPort, bootstrapNodeNetwork)
	bootstrapNodeNetwork.SetDHTHandler(bootstrapNode)

	go bootstrapNodeNetwork.Listen(ctx)

	testNodes := make([]*Node, clusterSize)
	testNetworks := make([]*QUICNetwork, clusterSize)
	testPorts := make([]int, clusterSize)

	// initialize nodes
	for i := range clusterSize {
		var err error
		testPorts[i] = bootstrapPort + i + 1
		testNetworks[i], err = NewQUICNetwork(udpIp, testPorts[i])
		if err != nil {
			t.Fatal(err)
		}
		testNodes[i] = NewNode(ctx, NewRandomId(), udpIp, testPorts[i], testNetworks[i])
		testNetworks[i].SetDHTHandler(testNodes[i])

		go testNetworks[i].Listen(ctx)
	}

	for i := range clusterSize {
		//	start := time.Now()
		err := testNodes[i].Join(ctx, bootstrapNode.Self)
		if err != nil {
			t.Fatalf("error joining bootstrapNode of node id: %v, address: %v:%v", testNodes[i].Self.ID,
				testNodes[i].Self.IP, testNodes[i].Self.Port)
		}
		//	elapsed := time.Now().Sub(start)
		//		t.Log("*****")
		//		t.Logf("elapsed time for node %v to join is %v", i, elapsed)
		//		t.Log("*****")
	}

	key := "hello"
	value := "world"

	nodeToPut := testNodes[rand.IntN(clusterSize)]
	nodeToGet := testNodes[rand.IntN(clusterSize)]
	nodeToPut.Put(ctx, key, []byte(value))

	// verify the value is retrievable
	retrievedValue, err := nodeToGet.Get(ctx, key)
	if err != nil {
		t.Fatalf("error getting value: %v", err)
	}
	expectedEntry := fmt.Sprintf("%s:%s", nodeToPut.Self.ID.String(), value)
	if !strings.Contains(string(retrievedValue), expectedEntry) {
		t.Fatalf("expected value to include %s, got %s", expectedEntry, string(retrievedValue))
	}
}

func Benchmark_ClusterSizeNPutAndGetKVPAirUDP(b *testing.B) {
	clusterSize := 1000
	udpIp := net.IPv4(127, 0, 0, 1)
	bootstrapPort := 10000

	ctx := context.Background()

	bootstrapNodeNetwork, err := NewUDPNetwork(udpIp, bootstrapPort)
	if err != nil {
		b.Fatalf("error creating bootstrap node: %v", err)
	}
	bootstrapNode := NewNode(ctx, NodeId{}, udpIp, bootstrapPort, bootstrapNodeNetwork)
	bootstrapNodeNetwork.SetDHTHandler(bootstrapNode)

	go bootstrapNodeNetwork.Listen()

	testNodes := make([]*Node, clusterSize)
	testNetworks := make([]*UDPNetwork, clusterSize)
	testPorts := make([]int, clusterSize)

	// initialize nodes
	for i := range clusterSize {
		var err error
		testPorts[i] = bootstrapPort + i + 1
		testNetworks[i], err = NewUDPNetwork(udpIp, testPorts[i])
		if err != nil {
			b.Fatal(err)
		}
		testNodes[i] = NewNode(ctx, NewRandomId(), udpIp, testPorts[i], testNetworks[i])
		testNetworks[i].SetDHTHandler(testNodes[i])

		go testNetworks[i].Listen()
	}

	for i := range clusterSize {
		//	start := time.Now()
		err := testNodes[i].Join(ctx, bootstrapNode.Self)
		if err != nil {
			b.Fatalf("error joining bootstrapNode of node id: %v, address: %v:%v", testNodes[i].Self.ID,
				testNodes[i].Self.IP, testNodes[i].Self.Port)
		}
		//	elapsed := time.Now().Sub(start)
		//		t.Log("*****")
		//		t.Logf("elapsed time for node %v to join is %v", i, elapsed)
		//		t.Log("*****")
	}

	key := "hello"
	value := "world"

	nodeToPut := testNodes[rand.IntN(clusterSize)]
	nodeToGet := testNodes[rand.IntN(clusterSize)]
	for b.Loop() {
		nodeToPut.Put(ctx, key, []byte(value))
		nodeToGet.Get(ctx, key)
	}

	// verify the value is retrievable
}
