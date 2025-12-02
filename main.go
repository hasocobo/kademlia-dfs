package main

import (
	"flag"
	"net"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
)

func main() {

	ipPtr := flag.String("ip", "127.0.0.1", "usage: -ip=127.0.0.1")
	portPtr := flag.Int("port", 9999, "-port=9999")
	bootstrapNodeIpPtr := flag.String("bootstrap-ip", "127.0.0.1", "usage: -ip=127.0.0.1")
	bootstrapNodePortPtr := flag.Int("bootstrap-port", 9999, "-port=9999")
	flag.Parse()

	ipAddress := net.ParseIP(*ipPtr)
	port := *portPtr
	bootstrapNodeIpAddress := net.ParseIP(*bootstrapNodeIpPtr)
	bootstrapNodePort := *bootstrapNodePortPtr

	udpNetwork := kademliadfs.NewUDPNetwork(ipAddress, port)
	node := kademliadfs.NewNode(kademliadfs.NewRandomId(), ipAddress, port, udpNetwork)
	node.Join(kademliadfs.Contact{IP: bootstrapNodeIpAddress, Port: bootstrapNodePort}) // TODO: ID is not known so we need to call ping first

	udpNetwork.SetHandler(node)

	go udpNetwork.Listen()
	select {} // Block main to keep the program alive to run goroutines

}
