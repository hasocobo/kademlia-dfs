package scheduler

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
	"github.com/hasocobo/kademlia-dfs/runtime"
)

const (
	taskTimeoutSeconds = 30
)

type TaskDescription struct {
	ID        TaskID
	JobID     JobID
	Name      string
	Topic     string
	Type      runtime.TaskType
	TaskState TaskState

	Binary   []byte
	ChunkID  int
	Stdin    []byte
	Metadata []byte

	LeaseUntil   time.Time
	BackoffUntil time.Time // if a task dispatch is not successful, retry again when time.Now() > BackoffUntil
	EnqueuedAt   time.Time
	DispatchedAt time.Time
	DoneAt       time.Time

	Attempts int
	Queued   bool // is in the readyTaskQueue?
}

func (ts TaskState) String() string {
	return []string{"PENDING", "RUNNING", "DONE"}[ts]
}

type TaskHandler struct {
	scheduler *Scheduler
	worker    *Worker
}

func NewTaskHandler(sched *Scheduler, worker *Worker) TaskHandler {
	return TaskHandler{scheduler: sched, worker: worker}
}

func (th TaskHandler) HandleMessage(ctx context.Context, message []byte) ([]byte, error) {
	task, err := runtime.DecodeTask(message)
	if err != nil {
		return nil, fmt.Errorf("error decoding task: %v", err)
	}

	log.Printf("HandleMessage: got a request of type: %v", task.OpCode)

	switch task.OpCode {
	case kademliadfs.ExecutionResponse:
		th.scheduler.events <- EventTaskDone{taskID: task.TaskID, result: task.Result}
		return nil, nil

	case kademliadfs.LeaseRequest:
		ctx, cancel := context.WithTimeout(ctx, time.Second*taskTimeoutSeconds)
		defer cancel()

		return th.scheduler.HandleTaskLeaseRequest(ctx, task)

	default:
		return nil, fmt.Errorf("unknown message type")
	}
}

func (s *Scheduler) markTaskDispatched(taskID TaskID) error {
	task, ok := s.Tasks[taskID]
	if !ok {
		return fmt.Errorf("markTaskDispatched: task %v not found", taskID)
	}

	if task.TaskState == StateRunning {
		return nil
	}
	if task.TaskState == StateDone {
		return fmt.Errorf("markTaskDispatched: cannot transition state from: %v to %v", task.TaskState, StateRunning)
	}

	task.Queued = false
	task.TaskState = StateRunning
	task.LeaseUntil = time.Now().Add(time.Second * taskTTLSeconds)
	task.DispatchedAt = time.Now()

	s.stats.IncAssigned()
	s.stats.ObserveQueueWait(task.DispatchedAt.Sub(task.EnqueuedAt))

	return nil
}

func (s *Scheduler) markTaskDone(taskID TaskID, result []byte) error {
	task, ok := s.Tasks[taskID]
	if !ok {
		return fmt.Errorf("markTaskDone: task %v not found", taskID)
	}
	if task.TaskState == StateDone {
		return nil
	}

	if task.TaskState == StatePending {
		return fmt.Errorf("markTaskDone: cannot transition state from: %v to %v",
			task.TaskState, StateDone)
	}

	job, exists := s.Jobs[task.JobID]
	if !exists {
		return fmt.Errorf("job with jobID: %v does not exist", task.JobID)
	}

	writeErr := os.WriteFile(fmt.Sprintf("../.jobs/%v/%v.json", task.JobID, task.ChunkID), result, 0o644)
	if writeErr != nil {
		log.Printf("error writing plan: %v", writeErr)
		return writeErr
	}
	log.Println("successfully wrote the result")

	task.TaskState = StateDone
	task.Queued = false
	task.BackoffUntil = time.Time{}
	task.LeaseUntil = time.Time{}
	job.TasksDone++
	task.DoneAt = time.Now()

	s.stats.IncCompleted()
	s.stats.ObserveTaskRunDuration(task.DoneAt.Sub(task.DispatchedAt))

	log.Printf("job progress: %v/%v", job.TasksDone, job.TasksTotal)

	log.Printf("task: %v is of state: %v", task.Name, task.TaskState)
	if job.TasksDone == job.TasksTotal {
		s.markJobDone(job.ID)
	}
	return nil
}

func (s *Scheduler) markJobDone(jobID JobID) error {
	job, ok := s.Jobs[jobID]
	if !ok {
		return fmt.Errorf("markJobDone: job %v not found", jobID)
	}
	jobPath := jobDirPath + "/" + jobID.String()

	if err := os.MkdirAll(jobPath, 0o755); err != nil {
		log.Printf("error: failed to create job dir %s: %v", jobPath, err)
		return err
	}

	entries, err := os.ReadDir(jobPath)
	if err != nil {
		log.Printf("error: failed to read job dir %s: %v", jobPath, err)
		return err
	}

	var ndjson bytes.Buffer
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}

		name := ent.Name()
		log.Println(name)
		if name == "plan.json" || filepath.Ext(name) == ".wasm" {
			continue
		}

		filePath := jobPath + "/" + name
		b, err := os.ReadFile(filePath)
		if err != nil {
			log.Printf("error: failed to read file %s for job %s: %v", filePath, jobID.String(), err)
			return err
		}

		b = bytes.TrimSpace(b)
		if len(b) == 0 {
			continue
		}

		if _, err := ndjson.Write(b); err != nil {
			log.Printf("error: failed writing ndjson buffer for job %s (file %s): %v", jobID.String(), filePath, err)
			return err
		}
		if err := ndjson.WriteByte('\n'); err != nil {
			log.Printf("error: failed writing newline to ndjson buffer for job %s: %v", jobID.String(), err)
			return err
		}
	}

	log.Printf("job %s: reducer input size = %d", jobID.String(), ndjson.Len())
	res, err := s.planner.RunTask(context.TODO(), job.MergerBinary, ndjson.Bytes(), nil)
	if err != nil {
		log.Printf("error: running reducer wasm failed for job %s (%s): %v", jobID.String(), job.Name, err)
		return err
	}

	outPath := jobPath + "/output.json"
	if err := os.WriteFile(outPath, res, 0o644); err != nil {
		log.Printf("error: failed to write output file %s for job %s (%s): %v", outPath, jobID.String(), job.Name, err)
		return err
	}

	log.Printf("job %s (%s) wrote output successfully to %s", jobID.String(), job.Name, outPath)
	log.Printf("job %s (%s) has completed successfully", jobID.String(), job.Name)
	return nil
}

func (s *Scheduler) markTaskDispatchFailed(taskID TaskID) error {
	task, ok := s.Tasks[taskID]
	if !ok {
		return fmt.Errorf("markTaskDispatchFailed: task %v not found", taskID)
	}

	if task.TaskState == StatePending {
		return nil
	}

	if task.TaskState == StateDone {
		return fmt.Errorf("markTaskDispatchFailed: cannot transition state from: %v to %v", task.TaskState, StatePending)
	}

	task.TaskState = StatePending
	task.Attempts++
	task.BackoffUntil = time.Now().Add(calculateBackoffUntil(task.Attempts))
	task.LeaseUntil = time.Time{}
	task.Queued = false
	s.stats.IncDispatchFailed()

	log.Printf("task is backing off until: %v", task.BackoffUntil)

	return nil
}

func (s *Scheduler) enqueueTask(taskID TaskID) error {
	task, ok := s.Tasks[taskID]
	if !ok {
		return fmt.Errorf("enqueueTask: task %v not found", taskID)
	}

	if task.Queued {
		return nil
	}

	if task.TaskState == StateDone {
		return fmt.Errorf("enqueueTask: cannot transition state from: %v to %v", task.TaskState, StatePending)
	}

	if task.LeaseUntil.Compare(time.Now()) == -1 &&
		!task.Queued && task.BackoffUntil.Compare(time.Now()) == -1 {

		log.Printf("found a task: %v. scheduling for execution", task.Name)
		task.TaskState = StatePending
		task.Queued = true

		// if there's a pending request, answer it first

		if len(s.pendingLeaseRequests) > 0 {
			log.Printf("enqueueTask: serving %v pending requests first", len(s.pendingLeaseRequests))
			job, ok := s.Jobs[task.JobID]
			if !ok {
				return fmt.Errorf("enqueueTask: job %v not found", task.JobID)
			}

			for len(s.pendingLeaseRequests) > 0 {
				pendingLease := s.pendingLeaseRequests[0]
				s.pendingLeaseRequests = s.pendingLeaseRequests[1:]

				if pendingLease.ctx.Err() != nil {
					log.Printf("enqueueTask: lease expired, continuing with the next lease if available")
					continue
				}

				s.markTaskDispatched(task.ID)

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
				pendingLease.responseChan <- leaseResponse{payload: payload, err: err}
				log.Printf("enqueueTask: sent a task to the pending request's channel")
				return nil
			}
		}

		s.batchTasks <- taskID

		task.EnqueuedAt = time.Now()
		s.stats.IncEnqueued()
	}
	return nil
}

// calculateBackoffUntil calculates the time for the task to wait before getting rescheduled
func calculateBackoffUntil(attempt int) time.Duration {
	const max = 10 * time.Second

	if attempt <= 0 {
		return time.Second
	}

	if attempt >= 63 { // avoid overflow
		return max
	}

	base := time.Second * time.Duration(1<<uint(attempt))
	base = min(base, max)

	// jitter: random duration in [0, base)
	return time.Duration(rand.Int64N(int64(base)))
}
