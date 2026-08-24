package types

import (
	"sync"
	"testing"
)

func TestStreamRejectsFailedCandidate(t *testing.T) {
	stateSymbol := MustIntern("stream_test/state")
	inputSymbol := MustIntern("stream_test/input")
	primitive := func(input Frame) Frame {
		value := input.MustGet(inputSymbol)

		if value < 0 {
			input.Err = PrimitiveError("negative input")

			return input
		}

		input.Put(stateSymbol, input.MustGet(stateSymbol)+value)

		return input
	}

	initial := Frame{}
	initial.Put(stateSymbol, 1)
	stream := NewStream(primitive, initial)
	valid := Frame{}
	valid.Put(inputSymbol, 2)

	if output := stream.Step(valid); output.Err != nil {
		t.Fatal(output.Err)
	}

	invalid := Frame{}
	invalid.Put(inputSymbol, -4)

	if output := stream.Step(invalid); output.Err == nil {
		t.Fatal("negative input should fail")
	}

	if got := stream.Project().MustGet(stateSymbol); got != 3 {
		t.Fatalf("state=%v; want retained value 3", got)
	}
}

func TestAtomicStreamCommitsConcurrentTransitions(t *testing.T) {
	totalSymbol := MustIntern("atomic_stream_test/total")
	deltaSymbol := MustIntern("atomic_stream_test/delta")
	primitive := func(input Frame) Frame {
		total, _ := input.Get(totalSymbol)
		delta := input.MustGet(deltaSymbol)
		input.Put(totalSymbol, total+delta)

		return input
	}

	stream := NewAtomicStream(primitive, Frame{})
	input := Frame{}
	input.Put(deltaSymbol, 1)
	waitGroup := sync.WaitGroup{}

	for worker := 0; worker < 8; worker++ {
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for iteration := 0; iteration < 25; iteration++ {
				if output := stream.Step(input); output.Err != nil {
					t.Error(output.Err)
					return
				}
			}
		}()
	}

	waitGroup.Wait()

	if got := stream.Project().MustGet(totalSymbol); got != 200 {
		t.Fatalf("total=%v; want 200", got)
	}
}

func BenchmarkStreamStep(b *testing.B) {
	stateSymbol := MustIntern("stream_benchmark/state")
	inputSymbol := MustIntern("stream_benchmark/input")
	primitive := func(input Frame) Frame {
		input.Put(stateSymbol, input.MustGet(inputSymbol))

		return input
	}
	stream := NewStream(primitive, Frame{})
	input := Frame{}
	input.Put(inputSymbol, 1)

	b.ReportAllocs()

	for b.Loop() {
		_ = stream.Step(input)
	}
}

func BenchmarkAtomicStreamStep(b *testing.B) {
	stateSymbol := MustIntern("atomic_stream_benchmark/state")
	inputSymbol := MustIntern("atomic_stream_benchmark/input")
	primitive := func(input Frame) Frame {
		input.Put(stateSymbol, input.MustGet(inputSymbol))

		return input
	}
	stream := NewAtomicStream(primitive, Frame{})
	input := Frame{}
	input.Put(inputSymbol, 1)

	b.ReportAllocs()

	for b.Loop() {
		if output := stream.Step(input); output.Err != nil {
			b.Fatal(output.Err)
		}
	}
}
