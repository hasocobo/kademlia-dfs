package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
	"github.com/hasocobo/kademlia-dfs/scheduler"
)

type Server struct {
	storedBinaries map[string][]byte
	scheduler      *scheduler.Scheduler
	httpPort       int
	jobParser      scheduler.JobParser
}

func NewServer(scheduler *scheduler.Scheduler, httpPort int, jobParser scheduler.JobParser) *Server {
	return &Server{
		storedBinaries: map[string][]byte{
			"add":      wasmAdd,
			"subtract": wasmSubtract,
		},
		httpPort:  httpPort,
		scheduler: scheduler,
		jobParser: jobParser,
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
			if err := s.scheduler.Node.Put(ctx, kv.Key, []byte(kv.Value)); err != nil {
				log.Printf("error putting key value pair: %v\n", err)
				http.Error(w, "failed to store value", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)

		case http.MethodGet:
			log.Println("handling a get request")
			value, err := s.scheduler.Node.Get(ctx, kv.Key)
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

func (s *Server) handleJob() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var jobSpec scheduler.JobSpec

			s.jobParser.Parse(r.Body, &jobSpec)

			log.Println(jobSpec.ToString())

			w.WriteHeader(http.StatusOK)
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

		job := scheduler.JobDescription{
			ID:         scheduler.JobID(kademliadfs.NewRandomId()),
			Name:       binaryName,
			Binary:     binary,
			TasksTotal: 300,
		}
		s.scheduler.RegisterJob(job)
		w.WriteHeader(http.StatusOK)
	}
}

func (s *Server) ServeHTTP(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/kv", s.handleKV(ctx))
	mux.HandleFunc("/wasm/{binaryName}", s.handleWasm())
	mux.HandleFunc("/jobs", s.handleJob())

	addr := "127.0.0.1:" + strconv.Itoa(s.httpPort)
	log.Println("listening http on " + addr)
	return http.ListenAndServe(addr, mux)
}
