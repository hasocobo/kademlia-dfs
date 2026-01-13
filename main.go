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
	"os"
	"os/signal"
	"strconv"
	"syscall"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
	"github.com/hasocobo/kademlia-dfs/runtime"
)

//go:embed runtime/binaries/add.wasm
var wasmAdd []byte

//go:embed runtime/binaries/subtract.wasm
var wasmSubtract []byte

func main() {
	ipAddress, port, bootstrapNodeIP, bootstrapNodePort, isBootstrapNode := parseFlags()

	udpPort := port
	udpIP := ipAddress
	nodeId := kademliadfs.NewRandomId()

	if isBootstrapNode {
		udpPort = bootstrapNodePort
		udpIP = bootstrapNodeIP
		nodeId = kademliadfs.NodeId{}
	}

	ctx := context.Background()

	// udpNetwork, err := kademliadfs.NewUDPNetwork(udpIP, udpPort)
	udpNetwork, err := kademliadfs.NewQUICNetwork(udpIP, udpPort)
	if err != nil {
		log.Fatal(err)
	}

	go udpNetwork.Listen(ctx)
	//	if err := udpNetwork.SendSTUNRequest(ctx); err != nil {
	//		log.Print(err)
	//	}

	var node *kademliadfs.Node
	if udpNetwork.PublicAddr != nil {
		node = kademliadfs.NewNode(ctx, nodeId, udpNetwork.PublicAddr.IP, udpNetwork.PublicAddr.Port, udpNetwork)
	} else {
		localIP, err := kademliadfs.GetOutboundIP(ctx)
		if err != nil {
			log.Fatalf("error getting outbound ip: %v", err)
		}
		log.Println("failed determining public address, switching to local address")
		log.Println(localIP)
		node = kademliadfs.NewNode(ctx, nodeId, localIP, udpPort, udpNetwork)
	}

	udpNetwork.SetHandler(node)

	if !isBootstrapNode {
		bootstrapContact := kademliadfs.Contact{
			IP:   bootstrapNodeIP,
			Port: bootstrapNodePort,
			ID:   kademliadfs.NodeId{},
		}
		if err := node.Join(ctx, bootstrapContact); err != nil {
			log.Printf("failed to join network: %v", err)
		}
	}

	tcpNetwork := &runtime.TCPNetwork{}
	wasmRuntime, err := runtime.NewWasmRuntime(tcpNetwork)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		if err := wasmRuntime.Serve(fmt.Sprintf("0.0.0.0:%d", node.Self.Port+2000)); err != nil {
			log.Printf("wasm runtime server error: %v", err)
		}
	}()

	server := NewServer(node, wasmRuntime, port+1000)
	go func() {
		if err := server.ServeHTTP(ctx); err != nil {
			log.Printf("http server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")
}

type KV struct {
	Key   string
	Value string
}

type Server struct {
	node           *kademliadfs.Node
	wasmRuntime    *runtime.WasmRuntime
	storedBinaries map[string][]byte
	httpPort       int
}

func NewServer(node *kademliadfs.Node, wasmRuntime *runtime.WasmRuntime, httpPort int) *Server {
	return &Server{
		node:        node,
		wasmRuntime: wasmRuntime,
		storedBinaries: map[string][]byte{
			"add":      wasmAdd,
			"subtract": wasmSubtract,
		},
		httpPort: httpPort,
	}
}

func (s *Server) handleKV(ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var kv KV
		if err := json.NewDecoder(r.Body).Decode(&kv); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodPut:
			log.Println("handling a put request")
			if err := s.node.Put(ctx, kv.Key, []byte(kv.Value)); err != nil {
				log.Printf("error putting key value pair: %v\n", err)
				http.Error(w, "failed to store value", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)

		case http.MethodGet:
			log.Println("handling a get request")
			value, err := s.node.Get(ctx, kv.Key)
			if err != nil {
				log.Printf("error getting key value pair: %v\n", err)
				http.Error(w, "failed to retrieve value", http.StatusInternalServerError)
				return
			}
			kv.Value = string(value)
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(kv); err != nil {
				log.Printf("error encoding response: %v", err)
			}

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (s *Server) handleWasm() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		binaryName := r.PathValue("binaryName")
		log.Printf("sending task to other nodes: %v\n", r.URL)

		binary, exists := s.storedBinaries[binaryName]
		if !exists {
			http.Error(w, "binary not found", http.StatusNotFound)
			return
		}

		nodesToSendCode := s.node.RoutingTable.FindClosest(kademliadfs.NewRandomId(), 8)

		wt := runtime.WasmTask{
			WasmBinary:       binary,
			WasmBinaryLength: uint64(len(binary)),
		}

		encodedWasmTask, err := s.wasmRuntime.Encode(wt)
		if err != nil {
			log.Printf("error encoding wasm task: %v", err)
			http.Error(w, "failed to encode task", http.StatusInternalServerError)
			return
		}

		for _, contact := range nodesToSendCode {
			log.Printf("sending to %v:%v", contact.IP, contact.Port+2000)
			if err := s.wasmRuntime.SendTask(encodedWasmTask, fmt.Sprintf("%v:%v", contact.IP, contact.Port+2000)); err != nil {
				log.Printf("error sending task: %v", err)
			}
		}

		w.WriteHeader(http.StatusOK)
	}
}

func (s *Server) ServeHTTP(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/kv", s.handleKV(ctx))
	mux.HandleFunc("/wasm/{binaryName}", s.handleWasm())

	addr := "127.0.0.1:" + strconv.Itoa(s.httpPort)
	log.Println("listening http on " + addr)
	return http.ListenAndServe(addr, mux)
}

func parseFlags() (ip net.IP, port int, bootstrapIP net.IP, bootstrapPort int, isBootstrap bool) {
	ipPtr := flag.String("ip", "0.0.0.0", "usage: -ip=0.0.0.0")
	portPtr := flag.Int("port", 9999, "-port=9999")
	bootstrapNodeIpPtr := flag.String("bootstrap-ip", "0.0.0.0", "usage: -ip=0.0.0.0")
	bootstrapNodePortPtr := flag.Int("bootstrap-port", 9000, "-bootstrap-port=9000")
	isBootstrapNodePtr := flag.Bool("is-bootstrap", false, "is-bootstrap=false")
	flag.Parse()

	return net.ParseIP(*ipPtr), *portPtr, net.ParseIP(*bootstrapNodeIpPtr), *bootstrapNodePortPtr, *isBootstrapNodePtr
}
