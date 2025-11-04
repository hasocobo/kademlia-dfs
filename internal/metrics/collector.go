package metrics

import (
	"sync"
	"time"
)

// Collector collects and aggregates metrics
type Collector struct {
	mu            sync.RWMutex
	latency       *LatencyMetrics
	resilience    *ResilienceMetrics
	startTime     time.Time
	networkSize   int
}

// NewCollector creates a new metrics collector
func NewCollector() *Collector {
	return &Collector{
		latency:    NewLatencyMetrics(),
		resilience: NewResilienceMetrics(),
		startTime:  time.Now(),
	}
}

// RecordLookup records a lookup operation
func (c *Collector) RecordLookup(duration time.Duration) {
	c.latency.RecordLookup(duration)
}

// RecordRetrieval records a file retrieval attempt
func (c *Collector) RecordRetrieval(success bool) {
	c.resilience.RecordRetrieval(success)
}

// RecordNodeFailure records a node failure
func (c *Collector) RecordNodeFailure(nodeID string) {
	c.resilience.RecordNodeFailure(nodeID)
}

// RecordNodeDeparture records a node departure
func (c *Collector) RecordNodeDeparture(nodeID string) {
	c.resilience.RecordNodeDeparture(nodeID)
}

// SetNetworkSize sets the current network size
func (c *Collector) SetNetworkSize(size int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.networkSize = size
}

// GetNetworkSize returns the current network size
func (c *Collector) GetNetworkSize() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.networkSize
}

// GetLatencyMetrics returns latency metrics
func (c *Collector) GetLatencyMetrics() *LatencyMetrics {
	return c.latency
}

// GetResilienceMetrics returns resilience metrics
func (c *Collector) GetResilienceMetrics() *ResilienceMetrics {
	return c.resilience
}

// GetUptime returns the uptime since collection started
func (c *Collector) GetUptime() time.Duration {
	return time.Since(c.startTime)
}

// GetSummary returns a summary of all metrics
func (c *Collector) GetSummary() Summary {
	return Summary{
		Uptime:            c.GetUptime(),
		NetworkSize:       c.GetNetworkSize(),
		LatencyStats:      c.latency.GetLatencyStats(),
		ResilienceStats:   c.resilience.GetResilienceStats(),
	}
}

// Summary contains a summary of all metrics
type Summary struct {
	Uptime          time.Duration
	NetworkSize     int
	LatencyStats    LatencyStats
	ResilienceStats ResilienceStats
}

// Reset resets all metrics
func (c *Collector) Reset() {
	c.latency.Reset()
	c.resilience.Reset()
	c.startTime = time.Now()
}

