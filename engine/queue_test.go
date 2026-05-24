package engine

import (
	"context"
	"testing"

	"github.com/theapemachine/symm/config"
)

func TestMeasurementQueueBounded(t *testing.T) {
	config.System.MaxPendingPerSignal = 4096
	config.System.MaxPendingGlobal = 0

	queue := &MeasurementQueue{}

	for index := 0; index < 4096; index++ {
		if err := queue.Enqueue(Measurement{Confidence: float64(index)}); err != nil {
			t.Fatalf("unexpected enqueue error at %d: %v", index, err)
		}
	}

	if err := queue.Enqueue(Measurement{Confidence: 9999}); err == nil {
		t.Fatal("expected error when queue is full")
	}

	if queue.Stats().Dropped != 1 {
		t.Fatalf("expected one dropped measurement, got %d", queue.Stats().Dropped)
	}

	count := 0

	for range queue.Drain(context.Background()) {
		count++
	}

	if count != 4096 {
		t.Fatalf("expected 4096 queued measurements, got %d", count)
	}
}

func TestMeasurementQueueFIFOOrder(t *testing.T) {
	config.System.MaxPendingPerSignal = 4
	config.System.MaxPendingGlobal = 0

	queue := &MeasurementQueue{}

	for index := 0; index < 4; index++ {
		if err := queue.Enqueue(Measurement{Confidence: float64(index)}); err != nil {
			t.Fatalf("enqueue %d: %v", index, err)
		}
	}

	got := make([]float64, 0, 4)

	for measurement := range queue.Drain(context.Background()) {
		got = append(got, measurement.Confidence)
	}

	if len(got) != 4 {
		t.Fatalf("expected 4 measurements, got %d", len(got))
	}

	for index := 0; index < len(got); index++ {
		if got[index] != float64(index) {
			t.Fatalf("expected fifo order, got %v", got)
		}
	}
}

func TestMeasurementQueueGlobalCap(t *testing.T) {
	config.System.MaxPendingPerSignal = 4096
	config.System.MaxPendingGlobal = 2

	first := &MeasurementQueue{}
	second := &MeasurementQueue{}

	if err := first.Enqueue(Measurement{Confidence: 1}); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}

	if err := second.Enqueue(Measurement{Confidence: 2}); err != nil {
		t.Fatalf("second enqueue: %v", err)
	}

	if err := first.Enqueue(Measurement{Confidence: 3}); err == nil {
		t.Fatal("expected global cap error")
	}

	if first.Stats().Dropped != 1 {
		t.Fatalf("expected one dropped measurement, got %d", first.Stats().Dropped)
	}

	for range first.Drain(context.Background()) {
	}

	for range second.Drain(context.Background()) {
	}
}

func TestScannerEnqueueAndDrain(t *testing.T) {
	scanner := NewScanner(context.Background())

	if err := scanner.Enqueue(Measurement{Confidence: 0.5}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

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
