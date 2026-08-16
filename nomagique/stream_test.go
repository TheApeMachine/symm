package nomagique

import (
	"sync"
	"testing"
)

func TestStreamRejectsFailedCandidate(t *testing.T) {
	stateSymbol := MustIntern("stream_test/state")
	inputSymbol := MustIntern("stream_test/input")
	primitive := func(state Frame, input Frame) (Frame, Frame, error) {
		value := input.MustGet(inputSymbol)

		if value < 0 {
			return state, Frame{}, primitiveError("negative input")
		}

		nextState := state
		nextState.Put(stateSymbol, state.MustGet(stateSymbol)+value)

		return nextState, nextState, nil
	}

	initial := Frame{}
	initial.Put(stateSymbol, 1)
	stream := NewStream(primitive, initial)
	valid := Frame{}
	valid.Put(inputSymbol, 2)

	if _, err := stream.Step(valid); err != nil {
		t.Fatal(err)
	}

	invalid := Frame{}
	invalid.Put(inputSymbol, -4)

	if _, err := stream.Step(invalid); err == nil {
		t.Fatal("negative input should fail")
	}

	if got := stream.Project().MustGet(stateSymbol); got != 3 {
		t.Fatalf("state=%v; want retained value 3", got)
	}
}

func TestAtomicStreamCommitsConcurrentTransitions(t *testing.T) {
	totalSymbol := MustIntern("atomic_stream_test/total")
	deltaSymbol := MustIntern("atomic_stream_test/delta")
	primitive := func(state Frame, input Frame) (Frame, Frame, error) {
		total, _ := state.Get(totalSymbol)
		delta := input.MustGet(deltaSymbol)
		nextState := state
		nextState.Put(totalSymbol, total+delta)

		return nextState, nextState, nil
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
				if _, err := stream.Step(input); err != nil {
					t.Error(err)
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
	primitive := func(state Frame, input Frame) (Frame, Frame, error) {
		nextState := state
		nextState.Put(stateSymbol, input.MustGet(inputSymbol))

		return nextState, nextState, nil
	}
	stream := NewStream(primitive, Frame{})
	input := Frame{}
	input.Put(inputSymbol, 1)

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		_, _ = stream.Step(input)
	}
}

func BenchmarkAtomicStreamStep(b *testing.B) {
	stateSymbol := MustIntern("atomic_stream_benchmark/state")
	inputSymbol := MustIntern("atomic_stream_benchmark/input")
	primitive := func(state Frame, input Frame) (Frame, Frame, error) {
		nextState := state
		nextState.Put(stateSymbol, input.MustGet(inputSymbol))

		return nextState, nextState, nil
	}
	stream := NewAtomicStream(primitive, Frame{})
	input := Frame{}
	input.Put(inputSymbol, 1)

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := stream.Step(input); err != nil {
			b.Fatal(err)
		}
	}
}
