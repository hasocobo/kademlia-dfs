package scheduler

import (
	"context"
	"log"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
	"github.com/hasocobo/kademlia-dfs/runtime"
)

type Scheduler struct {
	Jobs  map[JobID]*Job
	Tasks map[TaskID]*runtime.Task

	taskRuntime runtime.TaskRuntime
}

func NewScheduler(taskRuntime runtime.TaskRuntime) *Scheduler {
	return &Scheduler{
		Jobs:        make(map[JobID]*Job),
		Tasks:       make(map[TaskID]*runtime.Task),
		taskRuntime: taskRuntime,
	}
}

type (
	JobID     = kademliadfs.NodeId
	TaskID    = kademliadfs.NodeId
	TaskState int
)

type Job struct {
	ID        JobID
	Name      string
	Binary    []byte
	InputFile []byte
}

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
	task := s.taskRuntime.DecodeTask(message)

	if task.BinaryLength != 0 {
		s.taskRuntime.RunTask(ctx, task.Binary)
	}
}

func (s *Scheduler) RegisterJob(ctx context.Context, job Job) error {
	return nil
}
