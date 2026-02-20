package scheduler

import (
	"context"
	"log"
	"net"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
	"github.com/hasocobo/kademlia-dfs/runtime"
)

const (
	execWorkerPoolSize   = 8
	resultWorkerPoolSize = 8
)

type Worker struct {
	exec    runtime.TaskRuntime
	network runtime.TaskNetwork

	execQueue   chan runtime.Task
	resultQueue chan ExecResult
}

func NewWorker(exec runtime.TaskRuntime, network runtime.TaskNetwork) *Worker {
	return &Worker{
		exec:        exec,
		network:     network,
		execQueue:   make(chan runtime.Task, execWorkerPoolSize),
		resultQueue: make(chan ExecResult, resultWorkerPoolSize),
	}
}

func (w *Worker) Start(ctx context.Context) {
	log.Printf("worker starting")
	w.startWorkers(ctx, execWorkerPoolSize, resultWorkerPoolSize)
	go w.fetchTasks(ctx)
}

type ExecResult struct {
	TaskID TaskID
	Result []byte
}

func (w *Worker) startWorkers(ctx context.Context, execPoolSize int, resultPoolSize int) {
	for range execPoolSize {
		go w.execTask(ctx)
	}
	for range resultPoolSize {
		go w.writeTaskResult(ctx)
	}
}

func (w *Worker) execTask(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-w.execQueue:
			result, err := w.exec.RunTask(ctx, task.Binary, task.Stdin)
			if err != nil {
				log.Printf("execTask: error: %v", err)
				continue
			}
			w.resultQueue <- ExecResult{TaskID: task.TaskID, Result: result}
		}
	}
}

func (w *Worker) writeTaskResult(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case result := <-w.resultQueue:
			payload := runtime.Task{
				OpCode: kademliadfs.TaskExecutionResponse,
				TaskID: result.TaskID,
				Result: result.Result,
			}
			encodedMessage, err := runtime.EncodeTask(payload)
			if err != nil {
				log.Printf("writeTaskResult: error encoding: %v", err)
				continue
			}
			addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9000}
			if _, err := w.network.SendTask(ctx, encodedMessage, addr); err != nil {
				log.Printf("writeTaskResult: error sending result: %v", err)
			}
		}
	}
}

func (w *Worker) fetchTasks(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			log.Println("fetchTasks: polling for available tasks")
			bootstrapNode := kademliadfs.Contact{ID: kademliadfs.NodeId{}, IP: net.IPv4(127, 0, 0, 1), Port: 9000}

			taskPayload := runtime.Task{
				OpCode: kademliadfs.TaskLeaseRequest,
			}

			encodedTask, err := runtime.EncodeTask(taskPayload)
			if err != nil {
				log.Printf("fetchTasks: error encoding: %v", err)
				return
			}
			addr := &net.UDPAddr{IP: bootstrapNode.IP, Port: bootstrapNode.Port}
			response, err := w.network.RequestTask(ctx, encodedTask, addr)
			if err != nil {
				log.Printf("fetchTasks: no available task found")
				continue
			}

			task, err := runtime.DecodeTask(response)
			if err != nil {
				log.Printf("fetchTasks: error decoding: %v", err)
				continue
			}

			w.execQueue <- task
		}
	}
}
