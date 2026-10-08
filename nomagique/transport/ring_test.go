package transport

import (
	"runtime"
	"testing"
	"unsafe"
)

func TestRingNextRetainsOrderAndBounds(t *testing.T) {
	op := NewRing(4)

	if err := op.Error(); err != nil {
		t.Fatalf("construction: %v", err)
	}

	ring := op.(*Ring)
	one, two, three, four := 1.0, 2.0, 3.0, 4.0

	for _, value := range []unsafe.Pointer{
		unsafe.Pointer(&one),
		unsafe.Pointer(&two),
		unsafe.Pointer(&three),
		unsafe.Pointer(&four),
	} {
		if !ring.push(value) {
			t.Fatal("push failed below capacity")
		}
	}

	if ring.push(unsafe.Pointer(&one)) {
		t.Fatal("full ring accepted another value")
	}

	for expected := 1; expected <= 4; expected++ {
		value, found := ring.pop()

		if !found {
			t.Fatalf("expected %d; ring was empty", expected)
		}

		if *(*float64)(value) != float64(expected) {
			t.Fatalf("value=%f; want %d", *(*float64)(value), expected)
		}
	}

	if _, found := ring.pop(); found {
		t.Fatal("empty ring produced a value")
	}
}

func TestRingRejectsNonPowerOfTwoCapacity(t *testing.T) {
	op := NewRing(3)

	if err := op.Error(); err == nil {
		t.Fatal("invalid capacity was accepted")
	}
}

func TestRingNextDrainsArrivals(t *testing.T) {
	op := NewRing(4)
	values := []float64{1, 2, 3}
	var collected []float64

	for out := range op.Next(NewValues(values...).Next(nil)) {
		collected = append(collected, *(*float64)(out))
	}

	if err := op.Error(); err != nil {
		t.Fatalf("next: %v", err)
	}

	if len(collected) != 3 {
		t.Fatalf("collected=%v; want three values", collected)
	}
}

func TestRingTransfersConcurrentSPSCSequence(t *testing.T) {
	const count = 10_000
	values := make([]float64, 64)
	for index := range values {
		values[index] = float64(index)
	}

	ring := NewRing(64).(*Ring)
	producerDone := make(chan struct{})

	go func() {
		defer close(producerDone)

		for value := 0; value < count; value++ {
			for !ring.push(unsafe.Pointer(&values[value%len(values)])) {
				runtime.Gosched()
			}
		}
	}()

	for expected := 0; expected < count; expected++ {
		for {
			value, found := ring.pop()

			if !found {
				runtime.Gosched()
				continue
			}

			if *(*float64)(value) != values[expected%len(values)] {
				t.Fatalf("value out of order")
			}

			break
		}
	}

	<-producerDone
}

func BenchmarkRingRoundTrip(b *testing.B) {
	ring := NewRing(2).(*Ring)
	value := 1.0

	b.ReportAllocs()

	for b.Loop() {
		if !ring.push(unsafe.Pointer(&value)) {
			b.Fatal("push failed")
		}

		if _, found := ring.pop(); !found {
			b.Fatal("pop failed")
		}
	}
}
