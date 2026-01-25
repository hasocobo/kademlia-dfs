package scheduler

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

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

const (
	StatePending TaskState = iota
	StateRunning
	StateDone
)

type JobDescription struct {
	ID         JobID
	Name       string
	Binary     []byte
	InputFile  []byte
	TasksDone  int
	TasksTotal int // TODO: replace this part with a job description language like GDL
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

func (s *Scheduler) HandleMessage(ctx context.Context, message []byte) ([]byte, error) {
	log.Println("handling the message in task handler")
	task := s.taskRuntime.DecodeTask(message)

	switch task.OpCode {
	case kademliadfs.TaskExecute:
		if len(task.Binary) != 0 {
			response, err := s.taskRuntime.RunTask(ctx, task.Binary)
			if err != nil {
				log.Printf("error executing task: %v", err)
				return nil, err
			}

			task.OpCode = kademliadfs.TaskResult
			task.Result = response
			return s.taskRuntime.EncodeTask(task) // TODO: add error checking
		}
	case kademliadfs.TaskResult:
		s.mu.Lock()
		defer s.mu.Unlock()
		taskDescription, _ := s.Tasks[task.TaskID]
		jobDescription, _ := s.Jobs[taskDescription.JobID]
		taskDescription.TaskState = StateDone
		jobDescription.TasksDone++
		log.Printf("job progress: %v/%v", jobDescription.TasksDone, jobDescription.TasksTotal)
		log.Printf("task: %v is of state: %v", taskDescription.Name, taskDescription.TaskState)
		if jobDescription.TasksDone == jobDescription.TasksTotal {
			log.Printf("job: %v has completed successfully", jobDescription.Name)
			delete(s.Jobs, jobDescription.ID)
		}
		return nil, nil

	default:
		return nil, fmt.Errorf("unknown message type")
	}

	return nil, fmt.Errorf("error handling the message")
}

func (s *Scheduler) RegisterJob(ctx context.Context, job JobDescription) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.Jobs[job.ID]; exists {
		return fmt.Errorf("job: %v already exists", job)
	}

	s.Jobs[job.ID] = &job

	taskIDs := make([]TaskID, 0, job.TasksTotal)

	for i := range job.TasksTotal {
		taskId := TaskID(kademliadfs.NewRandomId())

		s.Tasks[taskId] = &TaskDescription{
			ID:        taskId,
			JobID:     job.ID,
			Name:      fmt.Sprintf("%v-%d", job.Name, i),
			TaskState: StatePending,
		}

		taskIDs = append(taskIDs, taskId)
	}

	s.mu.Unlock()
	defer s.mu.Lock()

	for _, taskId := range taskIDs {
		select {
		case s.TaskQueue <- taskId:
		case <-ctx.Done():
			return ctx.Err()
		}
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

			nodeToSendCode := nodesToSendCode[0]

			taskPayload := runtime.Task{
				OpCode: kademliadfs.TaskExecute,
				TaskID: taskID,
				Binary: binary,
				TTL:    time.Second * 10,
			}

			encodedTask, err := s.taskRuntime.EncodeTask(taskPayload)
			if err != nil {
				log.Printf("error encoding wasm task: %v", err)
				break
			}
			addr := &net.UDPAddr{IP: nodeToSendCode.IP, Port: nodeToSendCode.Port}
			response, err := s.taskNetwork.SendTask(ctx, encodedTask, addr)
			if err != nil {
				log.Printf("error sending task: %v", err)
				break
			}

			if len(response) > 0 {
				if _, err := s.HandleMessage(ctx, response); err != nil {
					log.Printf("error handling the task response: %v", err)
					break
				}
			}

		}
	}
}
