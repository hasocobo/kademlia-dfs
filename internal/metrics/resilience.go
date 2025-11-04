package metrics

import (
	"sync"
	"time"
)

// ResilienceMetrics tracks system resilience during network churn
type ResilienceMetrics struct {
	mu                sync.RWMutex
	totalRetrievals   int64
	successfulRetrievals int64
	failedRetrievals  int64
	nodeFailures      int64
	churnEvents       []ChurnEvent
}

// ChurnEvent represents a node failure or departure event
type ChurnEvent struct {
	Timestamp time.Time
	NodeID    string
	Type      string // "failure" or "departure"
}

// NewResilienceMetrics creates a new resilience metrics tracker
func NewResilienceMetrics() *ResilienceMetrics {
	return &ResilienceMetrics{
		churnEvents: make([]ChurnEvent, 0),
	}
}

// RecordRetrieval records a file retrieval attempt
func (rm *ResilienceMetrics) RecordRetrieval(success bool) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.totalRetrievals++
	if success {
		rm.successfulRetrievals++
	} else {
		rm.failedRetrievals++
	}
}

// RecordNodeFailure records a node failure event
func (rm *ResilienceMetrics) RecordNodeFailure(nodeID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.nodeFailures++
	rm.churnEvents = append(rm.churnEvents, ChurnEvent{
		Timestamp: time.Now(),
		NodeID:    nodeID,
		Type:      "failure",
	})
}

// RecordNodeDeparture records a node departure event
func (rm *ResilienceMetrics) RecordNodeDeparture(nodeID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.churnEvents = append(rm.churnEvents, ChurnEvent{
		Timestamp: time.Now(),
		NodeID:    nodeID,
		Type:      "departure",
	})
}

// GetSuccessRate returns the success rate of retrievals
func (rm *ResilienceMetrics) GetSuccessRate() float64 {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.totalRetrievals == 0 {
		return 0.0
	}

	return float64(rm.successfulRetrievals) / float64(rm.totalRetrievals) * 100.0
}

// GetResilienceStats returns resilience statistics
func (rm *ResilienceMetrics) GetResilienceStats() ResilienceStats {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return ResilienceStats{
		TotalRetrievals:      rm.totalRetrievals,
		SuccessfulRetrievals: rm.successfulRetrievals,
		FailedRetrievals:     rm.failedRetrievals,
		SuccessRate:          rm.GetSuccessRate(),
		NodeFailures:         rm.nodeFailures,
		ChurnEvents:          len(rm.churnEvents),
	}
}

// ResilienceStats contains resilience statistics
type ResilienceStats struct {
	TotalRetrievals      int64
	SuccessfulRetrievals int64
	FailedRetrievals     int64
	SuccessRate          float64
	NodeFailures         int64
	ChurnEvents          int
}

// Reset clears all metrics
func (rm *ResilienceMetrics) Reset() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.totalRetrievals = 0
	rm.successfulRetrievals = 0
	rm.failedRetrievals = 0
	rm.nodeFailures = 0
	rm.churnEvents = make([]ChurnEvent, 0)
}

// GetChurnEvents returns all churn events
func (rm *ResilienceMetrics) GetChurnEvents() []ChurnEvent {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	events := make([]ChurnEvent, len(rm.churnEvents))
	copy(events, rm.churnEvents)
	return events
}

