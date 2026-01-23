package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strconv"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
	"github.com/hasocobo/kademlia-dfs/runtime"
)

type Server struct {
	node           *kademliadfs.Node
	wasmNetwork    runtime.WasmNetwork
	storedBinaries map[string][]byte
	httpPort       int
}

func NewServer(node *kademliadfs.Node,
	wasmNetwork runtime.WasmNetwork, httpPort int,
) *Server {
	return &Server{
		node: node,
		storedBinaries: map[string][]byte{
			"add":      wasmAdd,
			"subtract": wasmSubtract,
		},
		wasmNetwork: wasmNetwork,
		httpPort:    httpPort,
	}
}

type KV struct {
	Key   string
	Value string
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
		ctx := context.Background()
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

		encodedWasmTask, err := runtime.EncodeWasmTask(wt)
		if err != nil {
			log.Printf("error encoding wasm task: %v", err)
			http.Error(w, "failed to encode task", http.StatusInternalServerError)
			return
		}

		for _, contact := range nodesToSendCode {
			log.Printf("sending to %v:%v", contact.IP, contact.Port)
			addr := &net.UDPAddr{IP: contact.IP, Port: contact.Port}
			if err := s.wasmNetwork.SendTask(ctx, encodedWasmTask, addr); err != nil {
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
