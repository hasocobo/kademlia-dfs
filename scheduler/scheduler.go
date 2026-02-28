package scheduler

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
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

	events               chan Event
	leaseResponse        chan leaseResponse
	pendingLeaseRequests []leaseRequest // list of chans of leaseRespons to send the task. used for long polling

	batchTasks  chan TaskID
	streamTasks map[JobID]*JobDescription // stream tasks are long running tasks and should not be drained from a queue

	Node *kademliadfs.Node

	stats       Stats
	planner     runtime.TaskRuntime // only for planning, actual execution is in the worker package
	taskNetwork runtime.TaskNetwork
}

type leaseRequest struct {
	ctx          context.Context
	responseChan chan leaseResponse
	request      runtime.Task
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
		Jobs:        make(map[JobID]*JobDescription),
		Tasks:       make(map[TaskID]*TaskDescription),
		events:      make(chan Event, eventLoopBufferSize),
		batchTasks:  make(chan TaskID, taskQueueSize),
		streamTasks: make(map[JobID]*JobDescription),
		Node:        node,
		stats:       stats,
		taskNetwork: taskNetwork,
		planner:     planner,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	s.reloadMemoryOnStartup(ctx, os.DirFS(jobDirPath))
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
		log.Printf("checking for expired requests in the pendingLeaseRequests list...")
		if len(s.pendingLeaseRequests) > 0 {

			filtered := s.pendingLeaseRequests[:0]
			for _, req := range s.pendingLeaseRequests {
				if req.ctx.Err() == nil {
					filtered = append(filtered, req)
				} else {
					log.Printf("removing an expired request")
				}
			}

			s.pendingLeaseRequests = filtered
		}

		if len(s.Tasks) == 0 {
			break
		}

		log.Printf("checking for expired tasks in %v tasks...", len(s.Tasks))

		for taskID, task := range s.Tasks {
			if task.LeaseUntil.After(time.Now()) {
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
		case taskID := <-s.batchTasks:
			task, exists := s.Tasks[taskID]
			if !exists {
				e.request.responseChan <- leaseResponse{nil, fmt.Errorf("task %v not found", taskID)}
				break
			}
			job, exists := s.Jobs[task.JobID]
			if !exists {
				e.request.responseChan <- leaseResponse{nil, fmt.Errorf("job %v not found", task.JobID)}
				break
			}

			s.markTaskDispatched(taskID)

			responseTask := runtime.Task{
				OpCode: kademliadfs.LeaseResponse,
				TaskID: taskID,
				JobID:  task.JobID,
				Type:   task.Type,
				Topic:  task.Topic,
				Binary: job.Binary,
				Stdin:  task.Stdin,
			}

			payload, err := runtime.EncodeTask(responseTask)
			e.request.responseChan <- leaseResponse{payload, err}

		default:
			if e.request.request.RequestMode == runtime.RequestModeNeedBatch {
				log.Printf("no task available, adding the request to the pending requests list")
				s.pendingLeaseRequests = append(s.pendingLeaseRequests, leaseRequest{
					responseChan: e.request.responseChan,
					ctx:          e.request.ctx,
					request:      e.request.request,
				})
				break
			}
			for _, desc := range s.streamTasks {
				set := make(map[string]struct{}, len(e.request.request.Tags))
				for _, v := range e.request.request.Tags {
					set[v] = struct{}{}
				}

				for _, tag := range desc.TargetTags {
					if _, ok := set[tag]; ok {
						responseTask := runtime.Task{
							OpCode: kademliadfs.LeaseResponse,
							JobID:  desc.ID,
							Type:   runtime.TaskTypeStream,
							Topic:  desc.Topic,
							Binary: desc.Binary,
						}
						payload, err := runtime.EncodeTask(responseTask)
						if err != nil {
							return err
						}
						e.request.responseChan <- leaseResponse{payload, err}
						return nil
					}
				}
			}
		}
	case EventJobSubmitted:
		job := e.job
		log.Printf("job received: %v", job.Name)
		if _, exists := s.Jobs[job.ID]; exists {
			return fmt.Errorf("job: %v already exists", job)
		}

		s.Jobs[job.ID] = &job

		switch job.Type {
		case TypeStream:
			s.streamTasks[job.ID] = &job
			log.Println("job registered successfully")

		case TypeBatch:
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
		default:
			return fmt.Errorf("unsupported job type: %v", job.Type)
		}

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

func (s *Scheduler) HandleTaskLeaseRequest(ctx context.Context, task runtime.Task) ([]byte, error) {
	responseChan := make(chan leaseResponse, 1)
	s.events <- EventLeaseRequest{
		leaseRequest{responseChan: responseChan, ctx: ctx, request: task},
	}

	select {
	case r := <-responseChan:
		if r.err != nil {
			return nil, r.err
		}
		return r.payload, nil

	case <-ctx.Done():
		log.Print("timeout exceeded for HandleTaskLeaseRequest")
		return nil, ctx.Err()
	}
}

func (s *Scheduler) RegisterJob(job JobSpec) error {
	jobID := kademliadfs.NewRandomId()
	log.Println(job.TargetTags)
	var executionPlan ExecutionPlan
	var jd JobDescription

	var tasks []*TaskDescription

	if job.Type == TypeStream {
		if job.Binary == "" {
			return fmt.Errorf("stream job requires binary")
		}
		binary, err := os.ReadFile(job.Binary)
		if err != nil {
			log.Printf("error reading stream binary: %v", err)
			return err
		}

		jd = JobDescription{
			ID:         jobID,
			Name:       job.Name,
			Binary:     binary,
			TasksTotal: 1,
			Type:       job.Type,
			Topic:      job.Topic,
			TargetTags: job.TargetTags,
		}
		s.events <- EventJobSubmitted{job: jd}
		return nil
	}

	for name, taskSpec := range job.Tasks {
		switch name {
		case "split":

			if err := os.MkdirAll(jobDirPath+"/"+jobID.String(), 0o755); err != nil {
				log.Printf("error creating job dir: %v", err)
				return err
			}

			binary, err := os.ReadFile(taskSpec.Run)
			if err != nil {
				log.Printf("error reading binary: %v", err)
				return err
			}

			writeErr := os.WriteFile(jobDirPath+"/"+jobID.String()+"/plan.wasm", binary, 0o644)
			if writeErr != nil {
				log.Printf("error writing plan for persistance: %v", writeErr)
				return writeErr
			}

			plan, err := s.planner.RunTask(context.TODO(), binary, nil, nil)
			if err != nil {
				log.Printf("error running planner wasm: %v", err)
				return err
			}
			log.Println("successfully created the plan")

			writeErr = os.WriteFile(jobDirPath+"/"+jobID.String()+"/plan.json", plan, 0o644)
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

			writeErr := os.WriteFile(jobDirPath+"/"+jobID.String()+"/map.wasm", binary, 0o644)
			if writeErr != nil {
				log.Printf("error writing exec for persistance: %v", writeErr)
				return writeErr
			}

			jd = JobDescription{
				ID:         jobID,
				Name:       executionPlan.Name,
				Binary:     binary,
				TasksTotal: executionPlan.Total,
				Type:       TypeBatch,
				Topic:      job.Topic,
			}

			tasks = make([]*TaskDescription, executionPlan.Total)
			taskType := taskSpec.Type
			if taskType == "" {
				taskType = runtime.TaskTypeBatch
			}

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
					Topic: job.Topic,
					Type:  taskType,

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

			writeErr := os.WriteFile(jobDirPath+"/"+jobID.String()+"/reduce.wasm", binary, 0o644)
			if writeErr != nil {
				log.Printf("error writing merge for persistence: %v", writeErr)
				return writeErr
			}

			jd.MergerBinary = binary
			s.events <- EventJobSubmitted{job: jd, tasks: tasks}

		default:
		}
	}

	return nil
}

// reloadMemoryOnStartup reconstructs the scheduler's unfinished jobs and tasks
func (s *Scheduler) reloadMemoryOnStartup(ctx context.Context, fsys fs.FS) error {
	log.Printf("reloading scheduler memory from disk...")

	jobDirs, err := fs.ReadDir(fsys, ".")
	if err != nil {
		log.Printf("failed reading jobs root: %v", err)
		return err
	}

	log.Printf("found %d job directories", len(jobDirs))

	for _, jobDir := range jobDirs {
		if !jobDir.IsDir() {
			continue
		}

		jobName := jobDir.Name()
		log.Printf("checking job folder: %s", jobName)

		raw, err := hex.DecodeString(jobName)
		if err != nil || len(raw) != 32 {
			log.Printf("skipping invalid job folder: %s", jobName)
			continue
		}

		var jobID JobID
		copy(jobID[:], raw)

		files, err := fs.ReadDir(fsys, jobName)
		if err != nil {
			log.Printf("failed reading job folder %s: %v", jobName, err)
			return err
		}

		hasOutput := false
		existing := make(map[int]struct{})

		for _, f := range files {
			name := f.Name()

			if name == "output.json" {
				log.Printf("job %s already completed (output.json exists)", jobName)
				hasOutput = true
				break
			}

			if name == "plan.json" {
				continue
			}

			if strings.HasSuffix(name, ".json") {
				base := strings.TrimSuffix(name, ".json")
				i, err := strconv.Atoi(base)
				if err == nil {
					existing[i] = struct{}{}
					log.Printf("job %s: found completed chunk %d", jobName, i)
				}
			}
		}

		if hasOutput {
			continue
		}

		log.Printf("job %s incomplete, reconstructing...", jobName)

		planBytes, err := fs.ReadFile(fsys, path.Join(jobName, "plan.json"))
		if err != nil {
			log.Printf("failed reading plan.json for job %s: %v", jobName, err)
			return err
		}

		execBytes, err := fs.ReadFile(fsys, path.Join(jobName, "map.wasm"))
		if err != nil {
			log.Printf("failed reading map.wasm for job %s: %v", jobName, err)
			return err
		}
		mergeBytes, err := fs.ReadFile(fsys, path.Join(jobName, "reduce.wasm"))
		if err != nil {
			log.Printf("failed reading reduce.wasm for job %s: %v", jobName, err)
			return err
		}

		var executionPlan ExecutionPlan
		if err := json.Unmarshal(planBytes, &executionPlan); err != nil {
			log.Printf("failed unmarshaling plan for job %s: %v", jobName, err)
			return err
		}

		log.Printf("job %s plan loaded: %d total tasks", jobName, executionPlan.Total)

		jd := JobDescription{
			ID:           jobID,
			Name:         executionPlan.Name,
			TasksTotal:   executionPlan.Total,
			TasksDone:    len(existing),
			Binary:       execBytes,
			MergerBinary: mergeBytes,
			Type:         TypeBatch,
		}

		var tasks []*TaskDescription

		for i, t := range executionPlan.Tasks {
			if _, done := existing[i]; done {
				continue
			}

			stdinBytes, err := base64.StdEncoding.DecodeString(t.Stdin)
			if err != nil {
				log.Printf("failed decoding stdin for job %s task %d: %v", jobName, i, err)
				return err
			}

			taskID := kademliadfs.NewRandomId()

			td := &TaskDescription{
				ID:      taskID,
				JobID:   jobID,
				Name:    fmt.Sprintf("%v-%v", executionPlan.Name, t.ID),
				Type:    runtime.TaskTypeBatch,
				Stdin:   stdinBytes,
				ChunkID: t.ID,
			}

			log.Printf("job %s: recreating task %d -> new taskID %x", jobName, i, taskID)

			tasks = append(tasks, td)
		}

		if len(tasks) > 0 {
			log.Printf("job %s: submitting %d reconstructed tasks to event loop", jobName, len(tasks))
			s.events <- EventJobSubmitted{
				job:   jd,
				tasks: tasks,
			}
		} else {
			log.Printf("job %s: nothing to reconstruct, all tasks are done, executing merger", jobName)
			s.events <- EventJobSubmitted{job: jd}
			s.events <- EventJobDone{jobID: jobID}
		}
	}

	log.Printf("scheduler reload complete")
	return nil
}
