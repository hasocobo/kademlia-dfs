package scheduler

import (
	"fmt"
	"log"
	"time"
)

type TaskDescription struct {
	ID        TaskID
	JobID     JobID
	Name      string
	TaskState TaskState

	LeaseUntil time.Time

	BackoffUntil time.Time // if a task dispatch is not successful, retry again when time.Now() > BackoffUntil
	Attempts     int
	Queued       bool // is in the readyTaskQueue?
}

func (ts TaskState) String() string {
	return []string{"PENDING", "RUNNING", "DONE"}[ts]
}

func (s *Scheduler) markTaskDispatched(taskID TaskID) error {
	task := s.mustTask(taskID)

	if task.TaskState == StateRunning {
		return nil
	}
	if task.TaskState == StateDone {
		return fmt.Errorf("markTaskDispatched: cannot transition state from: %v to %v", task.TaskState, StateRunning)
	}

	task.Queued = false
	task.TaskState = StateRunning
	task.LeaseUntil = time.Now().Add(time.Second * taskTTLSeconds)

	return nil
}

func (s *Scheduler) markTaskDone(taskID TaskID) error {
	task := s.mustTask(taskID)

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

	task.TaskState = StateDone
	task.Queued = false
	task.BackoffUntil = time.Time{}
	task.LeaseUntil = time.Time{}
	job.TasksDone++
	log.Printf("job progress: %v/%v", job.TasksDone, job.TasksTotal)

	log.Printf("task: %v is of state: %v", task.Name, task.TaskState)
	if job.TasksDone == job.TasksTotal {
		job := s.mustJob(task.JobID)
		log.Printf("job: %v has completed successfully", job.Name)
	}
	return nil
}

func (s *Scheduler) markTaskDispatchFailed(taskID TaskID) error {
	task := s.mustTask(taskID)

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

	log.Printf("task is backing off until: %v", task.BackoffUntil)

	return nil
}

func (s *Scheduler) enqueueTask(taskID TaskID) error {
	task := s.mustTask(taskID)

	if task.Queued {
		return nil
	}

	if task.TaskState == StateDone {
		return fmt.Errorf("enqueueTask: cannot transition state from: %v to %v", task.TaskState, StatePending)
	}

	if task.LeaseUntil.Compare(time.Now()) == -1 &&
		task.TaskState == StatePending &&
		!task.Queued && task.BackoffUntil.Compare(time.Now()) == -1 {
		log.Printf("found an expired task: %v. rescheduling for execution",
			task.Name)
		task.TaskState = StatePending
		task.Queued = true
		s.readyTaskQueue <- taskID
	}
	return nil
}

func (s *Scheduler) mustTask(taskID TaskID) *TaskDescription {
	task, exists := s.Tasks[taskID]
	if !exists {
		panic(fmt.Sprintf("task: %v not found", taskID))
	}
	return task
}

func (s *Scheduler) mustJob(jobID JobID) *JobDescription {
	job, exists := s.Jobs[jobID]
	if !exists {
		panic(fmt.Sprintf("job: %v not found", jobID))
	}
	return job
}

// calculateBackoffUntil calculates the time for the task to wait before getting rescheduled
func calculateBackoffUntil(attempt int) time.Duration {
	const max = 10 * time.Second

	if attempt <= 0 {
		return 1 * time.Second
	}

	if attempt >= 63 { // avoid overflow
		return max
	}

	d := time.Second * time.Duration(1<<uint(attempt))
	if d > max {
		return max
	}
	return d
}
