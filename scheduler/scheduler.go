package scheduler

import (
	"context"
	"log"

	"github.com/hasocobo/kademlia-dfs/runtime"
)

type Scheduler struct{}

type TaskState int

const (
	StatePending TaskState = iota
	StateLeased
	StateDone
)

func (ts TaskState) String() string {
	return []string{"PENDING", "LEASED", "DONE"}[ts]
}

func (s *Scheduler) HandleMessage(ctx context.Context, message []byte) {
	log.Println("handling the message in task handler")
	wasmTask := runtime.DecodeWasmTask(message)

	if wasmTask.WasmBinaryLength != 0 {
		runtime.RunWasmTask(wasmTask.WasmBinary)
	}
}
