package transport

import (
	"runtime"
	"testing"
)

func TestRingBufferRetainsOrderAndBounds(t *testing.T) {
	ring := MustNewRingBuffer[int](4)

	for value := 1; value <= 4; value++ {
		if !ring.Push(value) {
			t.Fatalf("push %d failed", value)
		}
	}

	if ring.Push(5) {
		t.Fatal("full ring accepted another value")
	}

	for expected := 1; expected <= 4; expected++ {
		value, found := ring.Pop()

		if !found || value != expected {
			t.Fatalf("value=%d found=%v; want %d", value, found, expected)
		}
	}

	if _, found := ring.Pop(); found {
		t.Fatal("empty ring produced a value")
	}
}

func TestRingBufferTransfersConcurrentSPSCSequence(t *testing.T) {
	const count = 10_000
	ring := MustNewRingBuffer[int](64)
	producerDone := make(chan struct{})

	go func() {
		defer close(producerDone)

		for value := 0; value < count; value++ {
			for !ring.Push(value) {
				runtime.Gosched()
			}
		}
	}()

	for expected := 0; expected < count; expected++ {
		for {
			value, found := ring.Pop()

			if !found {
				runtime.Gosched()
				continue
			}

			if value != expected {
				t.Fatalf("value=%d; want %d", value, expected)
			}

			break
		}
	}

	<-producerDone
}

func BenchmarkRingBufferRoundTrip(b *testing.B) {
	ring := MustNewRingBuffer[int](2)

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		if !ring.Push(1) {
			b.Fatal("push failed")
		}

		if _, found := ring.Pop(); !found {
			b.Fatal("pop failed")
		}
	}
}
