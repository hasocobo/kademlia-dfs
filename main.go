package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
)

func main() {
	ipPtr := flag.String("ip", "127.0.0.1", "usage: -ip=127.0.0.1")
	portPtr := flag.Int("port", 9999, "-port=9999")
	bootstrapNodeIpPtr := flag.String("bootstrap-ip", "0.0.0.0", "usage: -ip=0.0.0.0")
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

	udpNetwork, err := kademliadfs.NewUDPNetwork(udpIp, udpPort)
	if err != nil {
		log.Fatal(err)
	}
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
				} else if r.Method == "GET" {
					var kv KV
					log.Println("handling a get request")
					json.NewDecoder(r.Body).Decode(&kv)
					value, err := node.Get(kv.Key)
					if err != nil {
						log.Printf("error putting key value pair: %v \n", err)
						w.WriteHeader(500)
					}
					kv.Value = string(value[:])
					w.WriteHeader(200)
					resp, err := json.Marshal(kv)
					if err != nil {
						log.Printf("error marshaling key value pair :%v", err)
					}
					w.Write(resp)
				}
			})
			log.Println("listening on 0.0.0.0:8080")
			log.Fatal(http.ListenAndServe("0.0.0.0:8080", nil))
		}()
	}
	select {} // Block main to keep the program alive to run goroutines
}
