package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
)

func main() {
	ipPtr := flag.String("ip", "127.0.0.1", "usage: -ip=127.0.0.1")
	portPtr := flag.Int("port", 9999, "-port=9999")
	bootstrapNodeIpPtr := flag.String("bootstrap-ip", "127.0.0.1", "usage: -ip=127.0.0.1")
	bootstrapNodePortPtr := flag.Int("bootstrap-port", 9000, "-port=9000")
	isBootstrapNodePtr := flag.Bool("is-bootstrap", false, "is-bootsrap=false")
	flag.Parse()

	ipAddress := net.ParseIP(*ipPtr)
	port := *portPtr
	bootstrapNodeIpAddress := net.ParseIP(*bootstrapNodeIpPtr)
	bootstrapNodePort := *bootstrapNodePortPtr
	isBootstrapNode := *isBootstrapNodePtr

	udpPort := port
	udpIp := ipAddress
	if isBootstrapNode {
		udpPort = bootstrapNodePort
		udpIp = bootstrapNodeIpAddress
	}

	udpNetwork := kademliadfs.NewUDPNetwork(udpIp, udpPort)
	var node *kademliadfs.Node

	if isBootstrapNode {
		node = kademliadfs.NewNode(kademliadfs.NodeId{}, udpIp, udpPort, udpNetwork)
	} else {
		node = kademliadfs.NewNode(kademliadfs.NewRandomId(), udpIp, udpPort, udpNetwork)
		// TODO: ID is not known so we need to call ping first
	}
	udpNetwork.SetHandler(node)
	go udpNetwork.Listen()

	if !isBootstrapNode {
		node.Join(kademliadfs.Contact{IP: bootstrapNodeIpAddress, Port: bootstrapNodePort, ID: kademliadfs.NodeId{}})
	}

	type KV struct {
		Key   string
		Value string
	}
	if isBootstrapNode {
		go func() {
			http.HandleFunc("/kv", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PUT" {
					var kv KV
					log.Println("handling a put request")
					json.NewDecoder(r.Body).Decode(&kv)
					err := node.Put(kv.Key, []byte(kv.Value))
					if err != nil {
						log.Printf("error putting key value pair: %v \n", err)
						w.WriteHeader(500)
					}
					w.WriteHeader(201)
				}
			})
			log.Println("listening on 127.0.0.1:8080")
			log.Fatal(http.ListenAndServe("127.0.0.1:8080", nil))
		}()
	}
	select {} // Block main to keep the program alive to run goroutines
}
