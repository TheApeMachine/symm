package nomagique

import (
	"sync"
	"testing"
)

func TestInternDeduplicatesNames(t *testing.T) {
	first := MustIntern("frame_test/value")
	second := MustIntern("frame_test/value")

	if first != second {
		t.Fatalf("first=%d second=%d; want one stable symbol", first, second)
	}

	name, found := SymbolName(first)

	if !found || name != "frame_test/value" {
		t.Fatalf("name=%q found=%v; want frame_test/value", name, found)
	}
}

func TestInternDeduplicatesConcurrentRegistration(t *testing.T) {
	const workers = 32
	symbols := make(chan Symbol, workers)
	waitGroup := sync.WaitGroup{}

	for worker := 0; worker < workers; worker++ {
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()
			symbols <- MustIntern("frame_test/concurrent")
		}()
	}

	waitGroup.Wait()
	close(symbols)
	var expected Symbol
	initialized := false

	for symbol := range symbols {
		if !initialized {
			expected = symbol
			initialized = true
			continue
		}

		if symbol != expected {
			t.Fatalf("symbol=%d; want stable symbol %d", symbol, expected)
		}
	}
}

func TestFramePutGetMergeAndDelete(t *testing.T) {
	leftSymbol := MustIntern("frame_test/left")
	rightSymbol := MustIntern("frame_test/right")
	frame := Frame{}
	frame.Put(leftSymbol, 3)

	if value, found := frame.Get(leftSymbol); !found || value != 3 {
		t.Fatalf("left=%v found=%v; want 3", value, found)
	}

	overlay := Frame{}
	overlay.Put(rightSymbol, 4)
	frame.Merge(overlay)

	if frame.Count() != 2 || frame.MustGet(rightSymbol) != 4 {
		t.Fatalf("count=%d right=%v; want 2 and 4", frame.Count(), frame.MustGet(rightSymbol))
	}

	frame.Delete(leftSymbol)

	if frame.Has(leftSymbol) || frame.Count() != 1 {
		t.Fatalf("left present=%v count=%d; want false and 1", frame.Has(leftSymbol), frame.Count())
	}
}

func TestPipeCommitsTransactionally(t *testing.T) {
	stateSymbol := MustIntern("frame_test/state")
	resultSymbol := MustIntern("frame_test/result")
	first := func(state Frame, input Frame) (Frame, Frame, error) {
		nextState := state
		nextState.Put(stateSymbol, input.MustGet(resultSymbol)+1)

		return nextState, input, nil
	}
	second := func(state Frame, input Frame) (Frame, Frame, error) {
		return state, Frame{}, primitiveError("forced failure")
	}

	initial := Frame{}
	input := Frame{}
	input.Put(resultSymbol, 2)
	nextState, _, err := Pipe(first, second)(initial, input)

	if err == nil {
		t.Fatal("pipeline should fail")
	}

	if !nextState.Equal(initial) {
		t.Fatal("failed pipeline changed committed state")
	}
}

func BenchmarkFrameGet(b *testing.B) {
	symbol := MustIntern("frame_benchmark/value")
	frame := Frame{}
	frame.Put(symbol, 7)

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		_, _ = frame.Get(symbol)
	}
}
