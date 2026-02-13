package scheduler

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Stats interface {
	// traffic
	IncEnqueued()
	IncAssigned()
	IncCompleted()

	// errors
	IncDispatchFailed()
	IncExecFailed()

	// latency
	ObserveQueueWait(d time.Duration)
	ObserveTaskRunDuration(d time.Duration)

	// saturation
	SetSaturation(pending, running, readyQueueLen int)
}

type PrometheusStats struct {
	// traffic
	TasksEnqueued  prometheus.Counter
	TasksAssigned  prometheus.Counter
	TasksCompleted prometheus.Counter

	// errors
	DispatchFailed prometheus.Counter
	ExecFailed     prometheus.Counter

	// latency
	TaskExecDuration  prometheus.Histogram
	TaskQueueDuration prometheus.Histogram

	// saturation
	ReadyQueueLen prometheus.Gauge
	TasksPending  prometheus.Gauge
	TasksRunning  prometheus.Gauge

	Saturation prometheus.Gauge
}

func NewPrometheusStats(reg prometheus.Registerer) *PrometheusStats {
	m := &PrometheusStats{
		TasksEnqueued: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tasks_enqueued_total",
			Help: "Total number of tasks enqueued",
		}),
		TasksAssigned: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tasks_assigned_total",
			Help: "Total number of tasks assigned to workers",
		}),
		TasksCompleted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "tasks_completed_total",
			Help: "Total number of tasks completed successfully",
		}),
		DispatchFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dispatch_failed_total",
			Help: "Total number of task dispatch failures",
		}),
		ExecFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "exec_failed_total",
			Help: "Total number of task execution failures",
		}),

		ReadyQueueLen: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ready_queue_length",
			Help: "Current length of the ready task queue",
		}),
		TasksPending: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "tasks_pending",
			Help: "Current number of pending tasks ",
		}),
		TasksRunning: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "tasks_running",
			Help: "Current number of running tasks",
		}),
		Saturation: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "saturation",
			Help: "Scheduler saturation (user-defined scale)",
		}),

		TaskExecDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "task_exec_duration_seconds",
			Help:    "Time from assignment to completion",
			Buckets: prometheus.DefBuckets,
		}),
		TaskQueueDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "task_queue_duration_seconds",
			Help:    "Time from enqueue to assignment (queue wait time)",
			Buckets: prometheus.DefBuckets,
		}),
	}

	reg.MustRegister(
		m.TasksEnqueued,
		m.TasksAssigned,
		m.TasksCompleted,
		m.DispatchFailed,
		m.ExecFailed,
		m.ReadyQueueLen,
		m.TasksPending,
		m.TasksRunning,
		m.Saturation,
		m.TaskExecDuration,
		m.TaskQueueDuration,
	)

	return m
}

func (m *PrometheusStats) IncEnqueued() {
	m.TasksEnqueued.Inc()
}

func (m *PrometheusStats) IncAssigned() {
	m.TasksAssigned.Inc()
}

func (m *PrometheusStats) IncCompleted() {
	m.TasksCompleted.Inc()
}

func (m *PrometheusStats) IncDispatchFailed() {
	m.DispatchFailed.Inc()
}

func (m *PrometheusStats) IncExecFailed() {
	m.ExecFailed.Inc()
}

func (m *PrometheusStats) ObserveQueueWait(d time.Duration) {
	m.TaskQueueDuration.Observe(d.Seconds())
}

func (m *PrometheusStats) ObserveTaskRunDuration(d time.Duration) {
	m.TaskExecDuration.Observe(d.Seconds())
}

func (m *PrometheusStats) SetSaturation(pending, running, readyQueueLen int) {
	m.TasksPending.Set(float64(pending))
	m.TasksRunning.Set(float64(running))
	m.ReadyQueueLen.Set(float64(readyQueueLen))
}
