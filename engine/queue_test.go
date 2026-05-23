package engine

import (
	"context"
	"testing"
)

func TestMeasurementQueueBounded(t *testing.T) {
	queue := &MeasurementQueue{}

	for index := 0; index < maxPendingMeasurements+10; index++ {
		queue.Enqueue(Measurement{Confidence: float64(index)})
	}

	count := 0

	for range queue.Drain(context.Background()) {
		count++
	}

	if count != maxPendingMeasurements {
		t.Fatalf("expected %d queued measurements, got %d", maxPendingMeasurements, count)
	}
}

func TestScannerEnqueueAndDrain(t *testing.T) {
	scanner := NewScanner(context.Background(), 0)
	scanner.Enqueue(Measurement{Confidence: 0.5})

	count := 0

	for measurement := range scanner.Measure(context.Background()) {
		if measurement.Confidence != 0.5 {
			t.Fatalf("unexpected measurement %+v", measurement)
		}

		count++
	}

	if count != 1 {
		t.Fatalf("expected one measurement, got %d", count)
	}
}
