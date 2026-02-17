package scheduler

import (
	"context"
	"net"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
	"github.com/hasocobo/kademlia-dfs/runtime"
)

type Worker struct {
	exec runtime.TaskRuntime
}

func NewWorker() *Worker {
	return &Worker{}
}

func (w *Worker) fetchTasks(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			bootstrapNode := kademliadfs.Contact{ID: kademliadfs.NodeId{}, IP: net.IPv4(127, 0, 0, 1), Port: 9000}
			_ = bootstrapNode
		}
	}
}
