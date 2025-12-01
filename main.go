package main

import (
	"flag"
	"net"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
)

func main() {

	ipPtr := flag.String("ip", "127.0.0.1", "usage: -ip=127.0.0.1")
	portPtr := flag.Int("port", 9999, "-port=9999")
	flag.Parse()

	ipAddress := net.ParseIP(*ipPtr)
	port := *portPtr

	udpNetwork := kademliadfs.NewUDPNetwork(ipAddress, port)

	go udpNetwork.Listen()
	select {} // Block main to keep the program alive to run goroutines

}
