package scheduler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
	"github.com/hasocobo/kademlia-dfs/runtime"
)

const (
	taskQueueSize       = 1024
	taskTTLSeconds      = 10
	tickIntervalSeconds = 2
	eventLoopBufferSize = 1024
	jobDirPath          = "../.jobs"
)

type Scheduler struct {
	Jobs  map[JobID]*JobDescription
	Tasks map[TaskID]*TaskDescription

	events chan Event

	readyTaskQueue chan TaskID

	Node *kademliadfs.Node

	stats Stats

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
	taskNetwork runtime.TaskNetwork, stats Stats,
) *Scheduler {
	return &Scheduler{
		Jobs:           make(map[JobID]*JobDescription),
		Tasks:          make(map[TaskID]*TaskDescription),
		events:         make(chan Event, eventLoopBufferSize),
		readyTaskQueue: make(chan TaskID, taskQueueSize),
		Node:           node,
		stats:          stats,
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
	log.Println("event loop is running")
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
		log.Printf("job received: %v", job.Name)
		if _, exists := s.Jobs[job.ID]; exists {
			return fmt.Errorf("job: %v already exists", job)
		}

		s.Jobs[job.ID] = &job

		for _, task := range e.tasks {
			if ctx.Err() != nil {
				break
			}
			s.Tasks[task.ID] = task
			if err := s.enqueueTask(task.ID); err != nil {
				log.Printf("error enqueueing task: %v", err.Error())
				break
			}
		}
		return nil

	case EventJobDone:
		err := s.markJobDone(e.jobID)
		if err != nil {
			return err
		}
		return nil

	case EventTaskDispatchFailed:
		log.Printf("error sending task: %v, checking for node health via ping...", e.err)

		s.markTaskDispatchFailed(e.taskID)

		// for now, I just remove node but in the future we might first mark it as suspect and then ping it
		log.Printf("node: %v is dead, removing from the network", e.contact.ID)
		s.Node.RoutingTable.Remove(e.contact)

	case EventTaskDone:
		err := s.markTaskDone(e.taskID, e.result) // NOTE: use task instead of taskID
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
			response, err := s.taskRuntime.RunTask(ctx, task.Binary, task.Stdin)
			if err != nil {
				log.Printf("error executing task: %v", err)
				return nil, err
			}

			task.OpCode = kademliadfs.TaskResult
			task.Result = response
			return s.taskRuntime.EncodeTask(task) // TODO: add error checking
		}
	case kademliadfs.TaskResult:
		s.events <- EventTaskDone{taskID: task.TaskID, result: task.Result}
		return nil, nil

	case kademliadfs.TaskLeaseRequest:

	case kademliadfs.TaskLeaseResponse:

	default:
		return nil, fmt.Errorf("unknown message type")
	}

	return nil, fmt.Errorf("error handling the message")
}

func (s *Scheduler) RegisterJob(job JobSpec) error {
	jobID := kademliadfs.NewRandomId()
	var executionPlan ExecutionPlan
	var jd JobDescription

	var tasks []*TaskDescription

	for name, taskSpec := range job.Tasks {
		switch name {
		case "split":
			if err := os.MkdirAll(jobDirPath+"/"+jobID.String(), 0o755); err != nil {
				log.Printf("error creating job dir: %v", err)
				return err
			}

			inputFile, err := os.ReadFile(taskSpec.Input)
			if err != nil {
				log.Printf("error reading file: %v", err)
				return err
			}

			binary, err := os.ReadFile(taskSpec.Run)
			if err != nil {
				log.Printf("error reading binary: %v", err)
				return err
			}

			plan, err := s.taskRuntime.RunTask(context.TODO(), binary, inputFile)
			if err != nil {
				log.Printf("error running planner wasm: %v", err)
				return err
			}
			log.Println("successfully created the plan")

			writeErr := os.WriteFile(jobDirPath+"/"+jobID.String()+"/plan.json", plan, 0o644)
			if writeErr != nil {
				log.Printf("error writing plan: %v", writeErr)
				return writeErr
			}
			log.Println("successfully wrote the plan")

		case "execute":
			planBytes, err := os.ReadFile(jobDirPath + "/" + jobID.String() + "/plan.json")
			if err != nil {
				log.Printf("error reading plan: %v", err)
				return err
			}

			err = json.Unmarshal(planBytes, &executionPlan)
			if err != nil {
				log.Printf("error unmarshaling plan: %v", err)
				return err
			}

			if executionPlan.Total != len(executionPlan.Tasks) {
				return fmt.Errorf("plan total mismatch: total=%d tasks=%d", executionPlan.Total, len(executionPlan.Tasks))
			} // TODO: think about this

			binary, err := os.ReadFile(taskSpec.Run)
			if err != nil {
				log.Printf("error reading binary: %v", err)
				return err
			}

			jd = JobDescription{
				ID:         jobID,
				Name:       executionPlan.Name,
				Binary:     binary,
				TasksTotal: executionPlan.Total,
			}

			tasks = make([]*TaskDescription, executionPlan.Total)

			for i, t := range executionPlan.Tasks {
				log.Println(t)
				stdinBytes, err := base64.StdEncoding.DecodeString(t.Stdin)
				if err != nil {
					return fmt.Errorf("task %v stdin base64 decode failed: %v", t.ID, err)
				}

				tasks[i] = &TaskDescription{
					ID:    kademliadfs.NewRandomId(),
					JobID: jobID,
					Name:  fmt.Sprintf("%v-%v", executionPlan.Name, t.ID),

					Stdin:   stdinBytes,
					ChunkID: t.ID,
				}
			}

		case "merge":
			binary, err := os.ReadFile(taskSpec.Run)
			if err != nil {
				log.Printf("error reading binary: %v", err)
				return err
			}
			jd.MergerBinary = binary
			s.events <- EventJobSubmitted{job: jd, tasks: tasks}

		default:
		}
	}

	return nil
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
					Stdin:  task.Stdin,
					Binary: binary,
					TTL:    time.Second * 20, // TODO: remove
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
