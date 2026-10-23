package metrics

import (
	"sync"
	"time"

	"coreip-server-inspector/internal/models"
)

// RingBuffer stores a bounded time-series of metrics in memory.
type RingBuffer struct {
	mu       sync.RWMutex
	capacity int
	data     []models.LiveMetricPoint
	head     int
	count    int
}

// NewRingBuffer allocates a ring buffer of fixed capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 120
	}
	return &RingBuffer{
		capacity: capacity,
		data:     make([]models.LiveMetricPoint, capacity),
	}
}

// Add pushes a new point into the circular buffer.
func (rb *RingBuffer) Add(pt models.LiveMetricPoint) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if pt.Timestamp.IsZero() {
		pt.Timestamp = time.Now()
	}

	rb.data[rb.head] = pt
	rb.head = (rb.head + 1) % rb.capacity
	if rb.count < rb.capacity {
		rb.count++
	}
}

// GetAll returns all stored points ordered chronologically from oldest to newest.
func (rb *RingBuffer) GetAll() []models.LiveMetricPoint {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	result := make([]models.LiveMetricPoint, rb.count)
	if rb.count == 0 {
		return result
	}

	start := 0
	if rb.count == rb.capacity {
		start = rb.head
	}

	for i := 0; i < rb.count; i++ {
		idx := (start + i) % rb.capacity
		result[i] = rb.data[idx]
	}
	return result
}

// GetLatest returns the most recently inserted metric point.
func (rb *RingBuffer) GetLatest() (models.LiveMetricPoint, bool) {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.count == 0 {
		return models.LiveMetricPoint{}, false
	}

	idx := (rb.head - 1 + rb.capacity) % rb.capacity
	return rb.data[idx], true
}

// Count returns the number of points currently retained.
func (rb *RingBuffer) Count() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}
