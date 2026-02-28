package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/hasocobo/kademlia-dfs/scheduler"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	scheduler *scheduler.Scheduler
	httpPort  int
	jobParser scheduler.JobParser
	reg       *prometheus.Registry
}

func NewServer(scheduler *scheduler.Scheduler, httpPort int,
	jobParser scheduler.JobParser, reg *prometheus.Registry,
) *Server {
	return &Server{
		httpPort:  httpPort,
		scheduler: scheduler,
		jobParser: jobParser,
		reg:       reg,
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

			log.Println(jobSpec)
			err := s.scheduler.RegisterJob(jobSpec)
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusBadRequest)
			}

			w.WriteHeader(http.StatusOK)
		}
	}
}

func (s *Server) ServeHTTP(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/kv", s.handleKV(ctx))
	mux.HandleFunc("/jobs", s.handleJob())

	mux.Handle("/metrics", promhttp.HandlerFor(s.reg, promhttp.HandlerOpts{}))

	addr := "127.0.0.1:" + strconv.Itoa(s.httpPort)
	log.Println("listening http on " + addr)
	return http.ListenAndServe(addr, mux)
}
