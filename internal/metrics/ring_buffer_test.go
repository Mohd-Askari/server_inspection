package metrics

import (
	"testing"
	"time"

	"coreip-server-inspector/internal/models"
)

func TestRingBuffer_FifoAndRollover(t *testing.T) {
	capacity := 3
	rb := NewRingBuffer(capacity)

	if rb.Count() != 0 {
		t.Errorf("expected count 0, got %d", rb.Count())
	}

	// Add 3 points
	rb.Add(models.LiveMetricPoint{CPUUtilizationPct: 10, Timestamp: time.Unix(100, 0)})
	rb.Add(models.LiveMetricPoint{CPUUtilizationPct: 20, Timestamp: time.Unix(200, 0)})
	rb.Add(models.LiveMetricPoint{CPUUtilizationPct: 30, Timestamp: time.Unix(300, 0)})

	if rb.Count() != 3 {
		t.Errorf("expected count 3, got %d", rb.Count())
	}

	all := rb.GetAll()
	if len(all) != 3 {
		t.Fatalf("expected 3 items, got %d", len(all))
	}
	if all[0].CPUUtilizationPct != 10 || all[1].CPUUtilizationPct != 20 || all[2].CPUUtilizationPct != 30 {
		t.Errorf("unexpected FIFO order: %v", all)
	}

	// Push 4th point: should overwrite the oldest (10)
	rb.Add(models.LiveMetricPoint{CPUUtilizationPct: 40, Timestamp: time.Unix(400, 0)})

	if rb.Count() != 3 {
		t.Errorf("capacity must remain capped at 3, got %d", rb.Count())
	}

	allRolled := rb.GetAll()
	if len(allRolled) != 3 {
		t.Fatalf("expected 3 items, got %d", len(allRolled))
	}
	if allRolled[0].CPUUtilizationPct != 20 || allRolled[1].CPUUtilizationPct != 30 || allRolled[2].CPUUtilizationPct != 40 {
		t.Errorf("unexpected rollover order: %v", allRolled)
	}

	latest, ok := rb.GetLatest()
	if !ok || latest.CPUUtilizationPct != 40 {
		t.Errorf("expected latest to be 40, got %v", latest)
	}
}

