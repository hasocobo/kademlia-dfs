package kademliadfs

import (
	"net"
	"testing"
)

func TestIntegration_NodesJoinAndStoreKVPAirUDP(t *testing.T) {
	clusterSize := 1000
	udpIp := net.IPv4(127, 0, 0, 1)
	bootstrapPort := 10000

	bootstrapNodeNetwork, err := NewUDPNetwork(udpIp, bootstrapPort)
	if err != nil {
		t.Fatalf("error creating bootstrap node: %v", err)
	}
	bootstrapNode := NewNode(NodeId{}, udpIp, bootstrapPort, bootstrapNodeNetwork)
	bootstrapNodeNetwork.SetHandler(bootstrapNode)

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
			t.Fatal(err)
		}
		testNodes[i] = NewNode(NewRandomId(), udpIp, testPorts[i], testNetworks[i])
		testNetworks[i].SetHandler(testNodes[i])

		go testNetworks[i].Listen()
	}

	for i := range clusterSize {
		//	start := time.Now()
		err := testNodes[i].Join(bootstrapNode.Self)
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

	nodeToTest := testNodes[clusterSize/2]

	nodeToTest.Put(key, []byte(value))

	// verify the value is retrievable
	retrievedValue, err := nodeToTest.Get(key)
	if err != nil {
		t.Fatalf("error getting value: %v", err)
	}
	if string(retrievedValue) != value {
		t.Fatalf("expected value %s, got %s", value, string(retrievedValue))
	}
}
