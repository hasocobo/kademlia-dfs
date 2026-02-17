package scheduler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
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

	events        chan Event
	leaseRequest  chan leaseRequest
	leaseResponse chan leaseResponse

	readyTaskQueue chan TaskID

	Node *kademliadfs.Node

	stats Stats

	planner     runtime.TaskRuntime // only for planning, actual execution is in the worker package
	taskNetwork runtime.TaskNetwork
}

type leaseRequest struct {
	response leaseResponse
}

type leaseResponse struct {
	payload []byte
	err     error
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

func NewScheduler(node *kademliadfs.Node, planner runtime.TaskRuntime,
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
		planner:        planner,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	go s.startTicker(ctx)
	go s.runLoop(ctx)
}

func (s *Scheduler) startTicker(ctx context.Context) {
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
}

func (s *Scheduler) runLoop(ctx context.Context) {
	log.Println("event loop is running")
	for {
		select {
		case event := <-s.events:
			log.Printf("I got an event of type: %T", event)
			s.handleEvent(ctx, event)
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
			if task.LeaseUntil.Compare(time.Now()) == 1 {
				continue
			}

			if task.TaskState == StateDone {
				continue
			}

			err := s.enqueueTask(taskID)
			if err != nil {
				log.Println(err.Error())
				break
			}
		}
	case EventLeaseRequest:
		select {
		case taskID := <-s.readyTaskQueue:
			task := s.mustTask(taskID)
			job := s.mustJob(task.JobID)

			s.markTaskDispatched(taskID)

			responseTask := runtime.Task{
				OpCode: kademliadfs.TaskLeaseResponse,
				TaskID: taskID,
				Binary: job.Binary,
				Stdin:  task.Stdin,
			}

			payload, err := runtime.EncodeTask(responseTask)
			e.response <- leaseResponse{payload, err}
		default:
			e.response <- leaseResponse{nil, fmt.Errorf("no tasks available")}
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

	case EventTaskDone:
		err := s.markTaskDone(e.taskID, e.result) // NOTE: use task instead of taskID
		if err != nil {
			return err
		}
		return nil
	}

	return nil
}

func (s *Scheduler) HandleTaskLeaseRequest(ctx context.Context) ([]byte, error) {
	response := make(chan leaseResponse)
	s.events <- EventLeaseRequest{response: response}

	r := <-response
	if r.err != nil {
		return nil, r.err
	}
	return r.payload, nil
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

			plan, err := s.planner.RunTask(context.TODO(), binary, inputFile)
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
