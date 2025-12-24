package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"strconv"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
	"github.com/hasocobo/kademlia-dfs/runtime"
)

//go:embed runtime/main.wasm
var wasmAdd []byte

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

	ctx := context.Background()

	udpNetwork, err := kademliadfs.NewUDPNetwork(udpIp, udpPort)
	if err != nil {
		log.Fatal(err)
	}
	var node *kademliadfs.Node

	if isBootstrapNode {
		node = kademliadfs.NewNode(ctx, kademliadfs.NodeId{}, udpIp, udpPort, udpNetwork)
	} else {
		node = kademliadfs.NewNode(ctx, kademliadfs.NewRandomId(), udpIp, udpPort, udpNetwork)
		// TODO: ID is not known so we need to call ping first
	}
	udpNetwork.SetHandler(node)
	go udpNetwork.Listen()

	if !isBootstrapNode {
		node.Join(ctx, kademliadfs.Contact{IP: bootstrapNodeIpAddress, Port: bootstrapNodePort, ID: kademliadfs.NodeId{}})
	}

	tcpNetwork := runtime.TCPNetwork{}
	wasmRuntime, _ := runtime.NewWasmRuntime(&tcpNetwork)

	go wasmRuntime.Serve(fmt.Sprintf("%v:%v", node.Self.IP, node.Self.Port+2000))

	type KV struct {
		Key   string
		Value string
	}

	go func() {
		http.HandleFunc("/kv", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "PUT" {
				var kv KV
				log.Println("handling a put request")
				json.NewDecoder(r.Body).Decode(&kv)
				err := node.Put(ctx, kv.Key, []byte(kv.Value))
				if err != nil {
					log.Printf("error putting key value pair: %v \n", err)
					w.WriteHeader(500)
				}
				w.WriteHeader(201)
			} else if r.Method == "GET" {
				var kv KV
				log.Println("handling a get request")
				json.NewDecoder(r.Body).Decode(&kv)
				value, err := node.Get(ctx, kv.Key)
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

		http.HandleFunc("/wasm", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "GET" {
				var wt runtime.WasmTask
				log.Printf("sending task to other nodes :%v\n", r.URL)
				nodesToSendCode := node.RoutingTable.FindClosest(kademliadfs.NewRandomId(), 8)
				wt = runtime.WasmTask{
					WasmBinary:       wasmAdd,
					WasmBinaryLength: uint64(len(wasmAdd)),
				}

				log.Printf("yo thyo the length is %v", uint64(len(wasmAdd)))
				encodedWasmTask, err := wasmRuntime.Encode(wt)
				if err != nil {
					log.Printf("error encoding wasm task: %v", err)
				}

				for _, node := range nodesToSendCode {
					log.Println(node)
					wasmRuntime.SendTask(encodedWasmTask, fmt.Sprintf("%v:%v", node.IP, node.Port+2000))
				}
				w.WriteHeader(200)
			}
		})
		log.Println("listening http on 127.0.0.1:" + strconv.Itoa((port + 1000)))
		log.Fatal(http.ListenAndServe("127.0.0.1:"+strconv.Itoa((port+1000)), nil))
	}()
	select {} // Block main to keep the program alive to run goroutines
}
