package scheduler

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"sync"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
	"github.com/hasocobo/kademlia-dfs/runtime"
)

const (
	taskQueueSize = 32
)

type Scheduler struct {
	Jobs  map[JobID]*JobDescription
	Tasks map[TaskID]*TaskDescription

	TaskQueue chan TaskID

	Node *kademliadfs.Node

	taskRuntime runtime.TaskRuntime
	taskNetwork runtime.TaskNetwork

	mu sync.Mutex
}

type (
	JobID     = kademliadfs.NodeId
	TaskID    = kademliadfs.NodeId
	TaskState int
)

type JobDescription struct {
	ID            JobID
	Name          string
	Binary        []byte
	InputFile     []byte
	NumberOfTasks int // TODO: replace this part with a job description language like GDL
}

func (job JobDescription) String() string {
	return fmt.Sprintf("ID: %v Name:%v", job.ID.String(), job.Name)
}

type TaskDescription struct {
	ID        TaskID
	JobID     JobID
	Name      string
	TaskState TaskState
}

const (
	StatePending TaskState = iota
	StateRunning
	StateDone
)

func (ts TaskState) String() string {
	return []string{"PENDING", "RUNNING", "DONE"}[ts]
}

func NewScheduler(node *kademliadfs.Node, taskRuntime runtime.TaskRuntime,
	taskNetwork runtime.TaskNetwork,
) *Scheduler {
	return &Scheduler{
		Jobs:        make(map[JobID]*JobDescription),
		Tasks:       make(map[TaskID]*TaskDescription),
		TaskQueue:   make(chan TaskID, taskQueueSize),
		Node:        node,
		taskNetwork: taskNetwork,
		taskRuntime: taskRuntime,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	go s.dispatchTasks(ctx)
}

func (s *Scheduler) HandleMessage(ctx context.Context, message []byte) {
	log.Println("handling the message in task handler")
	task := s.taskRuntime.DecodeTask(message)

	if task.BinaryLength != 0 {
		s.taskRuntime.RunTask(ctx, task.Binary)
	}
}

func (s *Scheduler) RegisterJob(ctx context.Context, job JobDescription) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, exists := s.Jobs[job.ID]
	if exists {
		return fmt.Errorf("job: %v already exists", job)
	}

	s.Jobs[job.ID] = &job

	for i := range job.NumberOfTasks {
		taskId := TaskID(kademliadfs.NewRandomId())
		newTask := &TaskDescription{
			ID:        taskId,
			JobID:     job.ID,
			Name:      fmt.Sprintf("%v-%d", job.Name, i),
			TaskState: StatePending,
		}
		s.Tasks[taskId] = newTask
		s.TaskQueue <- taskId // TODO: fix this
	}
	return nil
}

func (s *Scheduler) dispatchTasks(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case taskID := <-s.TaskQueue:
			nodesToSendCode := s.Node.RoutingTable.FindClosest(kademliadfs.NewRandomId(), 1)
			s.mu.Lock()
			task := s.Tasks[taskID]
			task.TaskState = StateRunning
			binary := s.Jobs[task.JobID].Binary
			s.mu.Unlock()

			nodeToSendCode := nodesToSendCode[rand.IntN(1)]

			taskPayload := runtime.Task{
				Binary:       binary,
				BinaryLength: uint64(len(binary)),
			}

			encodedTask, err := s.taskRuntime.EncodeTask(taskPayload)
			if err != nil {
				log.Printf("error encoding wasm task: %v", err)
				return
			}
			addr := &net.UDPAddr{IP: nodeToSendCode.IP, Port: nodeToSendCode.Port}
			if err := s.taskNetwork.SendTask(ctx, encodedTask, addr); err != nil {
				log.Printf("error sending task: %v", err)
				return
			}

		}
	}
}
