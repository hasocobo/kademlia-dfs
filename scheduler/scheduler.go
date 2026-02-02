package scheduler

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
	"github.com/hasocobo/kademlia-dfs/runtime"
)

const (
	taskQueueSize       = 1024
	taskTTLSeconds      = 10
	tickIntervalSeconds = 2
	eventLoopBufferSize = 512
)

type Scheduler struct {
	Jobs  map[JobID]*JobDescription
	Tasks map[TaskID]*TaskDescription

	events chan Event

	readyTaskQueue chan TaskID

	Node *kademliadfs.Node

	taskRuntime runtime.TaskRuntime
	taskNetwork runtime.TaskNetwork
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

func NewScheduler(node *kademliadfs.Node, taskRuntime runtime.TaskRuntime,
	taskNetwork runtime.TaskNetwork,
) *Scheduler {
	return &Scheduler{
		Jobs:           make(map[JobID]*JobDescription),
		Tasks:          make(map[TaskID]*TaskDescription),
		events:         make(chan Event, eventLoopBufferSize),
		readyTaskQueue: make(chan TaskID, taskQueueSize),
		Node:           node,
		taskNetwork:    taskNetwork,
		taskRuntime:    taskRuntime,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	go func() {
		t := time.NewTicker(tickIntervalSeconds * time.Second)
		defer t.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.events <- EventTick{}
			}
		}
	}()

	go s.runLoop(ctx)
}

func (s *Scheduler) runLoop(ctx context.Context) {
	log.Println("loop is running")
	for {
		select {
		case event := <-s.events:
			log.Printf("I got an event of type: %T", event)
			s.handleEvent(ctx, event)
			s.maybeDispatchTasks(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) handleEvent(ctx context.Context, event Event) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	switch e := event.(type) {
	case EventTick:
		if len(s.Tasks) == 0 {
			break
		}

		log.Printf("checking for expired tasks in %v tasks...", len(s.Tasks))

		for taskID, task := range s.Tasks {
			if task.LeaseUntil.Compare(time.Now()) == 1 || task.TaskState != StatePending {
				continue
			}
			err := s.enqueueTask(taskID)
			if err != nil {
				log.Println(err.Error())
				break
			}
		}

	case EventJobSubmitted:
		job := e.job
		log.Printf("job received: %v", job)
		if _, exists := s.Jobs[job.ID]; exists {
			return fmt.Errorf("job: %v already exists", job)
		}

		s.Jobs[job.ID] = &job

		taskIDs := make([]TaskID, 0, job.TasksTotal)

		for i := range job.TasksTotal {
			taskID := TaskID(kademliadfs.NewRandomId())

			s.Tasks[taskID] = &TaskDescription{
				ID:        taskID,
				JobID:     job.ID,
				Name:      fmt.Sprintf("%v-%d", job.Name, i),
				TaskState: StatePending,
			}

			taskIDs = append(taskIDs, taskID)
		}

		for _, taskID := range taskIDs {
			if ctx.Err() != nil {
				break
			}
			if err := s.enqueueTask(taskID); err != nil {
				log.Printf("error enqueueing task: %v", err.Error())
				break
			}
		}
		return nil

		//	case EventTaskDispatched:
		//		taskID := e.taskID
	case EventJobDone:
		jobID := e.jobID
		job, exists := s.Jobs[jobID]
		if !exists {
			log.Printf("job: %v was not found in the job list", job)
		}
		log.Printf("job: %v has completed successfully", job.Name)

	case EventTaskDispatchFailed:
		log.Printf("error sending task: %v, checking for node health via ping...", e.err)

		s.markTaskDispatchFailed(e.taskID)

		// for now, I just remove node but in the future we might first mark it as suspect and then ping it
		log.Printf("node: %v is dead, removing from the network", e.contact.ID)
		s.Node.RoutingTable.Remove(e.contact)

	case EventTaskDone:
		err := s.markTaskDone(e.taskID)
		if err != nil {
			return err
		}
		return nil
	}

	return nil
}

func (s *Scheduler) HandleMessage(ctx context.Context, message []byte) ([]byte, error) {
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
		s.events <- EventTaskDone{taskID: task.TaskID}
		return nil, nil

	default:
		return nil, fmt.Errorf("unknown message type")
	}

	return nil, fmt.Errorf("error handling the message")
}

func (s *Scheduler) RegisterJob(job JobDescription) {
	s.events <- EventJobSubmitted{job: job}
}

func (s *Scheduler) maybeDispatchTasks(ctx context.Context) {
	burstSize := 32 // will drain tasks as bursts to avoid spamming the workers with tasks
	for range burstSize {
		select {
		case <-ctx.Done():
			return
		case taskID := <-s.readyTaskQueue:
			nodesToSendCode := s.Node.RoutingTable.FindClosest(kademliadfs.NewRandomId(), 1)

			err := s.markTaskDispatched(taskID)
			if err != nil {
				log.Println(err.Error())
				return
			}

			task := s.mustTask(taskID)
			job, exists := s.Jobs[task.JobID]
			var binary []byte
			if exists {
				binary = job.Binary
			}
			if len(nodesToSendCode) == 0 {
				log.Println("no nodes found to send the tasks")
				return
			}
			nodeToSendCode := nodesToSendCode[0]

			go func(contact kademliadfs.Contact) {
				taskPayload := runtime.Task{
					OpCode: kademliadfs.TaskExecute,
					TaskID: taskID,
					Binary: binary,
					TTL:    time.Second * 10, // TODO: remove
				}

				encodedTask, err := s.taskRuntime.EncodeTask(taskPayload)
				if err != nil {
					log.Printf("error encoding wasm task: %v", err)
					return
				}
				addr := &net.UDPAddr{IP: nodeToSendCode.IP, Port: nodeToSendCode.Port}
				response, err := s.taskNetwork.SendTask(ctx, encodedTask, addr)
				if err != nil {
					s.events <- EventTaskDispatchFailed{
						taskID:  taskID,
						contact: nodeToSendCode,
						err:     err,
					}
					return
				}

				if len(response) > 0 {
					if _, err := s.HandleMessage(ctx, response); err != nil {
						log.Printf("error handling the task response: %v", err)
						return
					}
				}
			}(nodeToSendCode)

		default:
			return
		}
	}
}
