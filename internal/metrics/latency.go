package metrics

import (
	"sync"
	"time"
)

// LatencyMetrics tracks lookup latency
type LatencyMetrics struct {
	mu            sync.RWMutex
	lookupTimes   []time.Duration
	totalLookups  int64
	totalLatency  time.Duration
}

// NewLatencyMetrics creates a new latency metrics tracker
func NewLatencyMetrics() *LatencyMetrics {
	return &LatencyMetrics{
		lookupTimes: make([]time.Duration, 0),
	}
}

// RecordLookup records a lookup operation and its duration
func (lm *LatencyMetrics) RecordLookup(duration time.Duration) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.lookupTimes = append(lm.lookupTimes, duration)
	lm.totalLookups++
	lm.totalLatency += duration
}

// GetAverageLatency returns the average lookup latency
func (lm *LatencyMetrics) GetAverageLatency() time.Duration {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	if lm.totalLookups == 0 {
		return 0
	}
	return lm.totalLatency / time.Duration(lm.totalLookups)
}

// GetLatencyStats returns latency statistics
func (lm *LatencyMetrics) GetLatencyStats() LatencyStats {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	if len(lm.lookupTimes) == 0 {
		return LatencyStats{}
	}

	min := lm.lookupTimes[0]
	max := lm.lookupTimes[0]
	var sum time.Duration

	for _, d := range lm.lookupTimes {
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
		sum += d
	}

	avg := sum / time.Duration(len(lm.lookupTimes))

	return LatencyStats{
		Min:     min,
		Max:     max,
		Average: avg,
		Count:   len(lm.lookupTimes),
	}
}

// LatencyStats contains latency statistics
type LatencyStats struct {
	Min     time.Duration
	Max     time.Duration
	Average time.Duration
	Count   int
}

// Reset clears all metrics
func (lm *LatencyMetrics) Reset() {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.lookupTimes = make([]time.Duration, 0)
	lm.totalLookups = 0
	lm.totalLatency = 0
}

