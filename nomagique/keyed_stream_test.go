package nomagique

import (
	"sync"
	"testing"
)

func TestKeyedStreamsRetainIndependentState(t *testing.T) {
	totalSymbol := MustIntern("keyed_stream_test/total")
	deltaSymbol := MustIntern("keyed_stream_test/delta")
	primitive := func(state Frame, input Frame) (Frame, Frame, error) {
		delta := input.MustGet(deltaSymbol)
		total, _ := state.Get(totalSymbol)
		nextState := state
		nextState.Put(totalSymbol, total+delta)

		return nextState, nextState, nil
	}
	collection := NewKeyedStreams[string](primitive, nil)
	firstInput := Frame{}
	firstInput.Put(deltaSymbol, 2)
	secondInput := Frame{}
	secondInput.Put(deltaSymbol, 5)

	if _, err := collection.Step("A", firstInput); err != nil {
		t.Fatal(err)
	}

	if _, err := collection.Step("B", secondInput); err != nil {
		t.Fatal(err)
	}

	if _, err := collection.Step("A", firstInput); err != nil {
		t.Fatal(err)
	}

	firstState, found := collection.Project("A")

	if !found || firstState.MustGet(totalSymbol) != 4 {
		t.Fatalf("A total=%v found=%v; want 4", firstState.MustGet(totalSymbol), found)
	}

	secondState, found := collection.Project("B")

	if !found || secondState.MustGet(totalSymbol) != 5 {
		t.Fatalf("B total=%v found=%v; want 5", secondState.MustGet(totalSymbol), found)
	}
}

func TestKeyedStreamsAllowConcurrentDifferentKeys(t *testing.T) {
	totalSymbol := MustIntern("keyed_stream_concurrent_test/total")
	deltaSymbol := MustIntern("keyed_stream_concurrent_test/delta")
	primitive := func(state Frame, input Frame) (Frame, Frame, error) {
		total, _ := state.Get(totalSymbol)
		nextState := state
		nextState.Put(totalSymbol, total+input.MustGet(deltaSymbol))

		return nextState, nextState, nil
	}
	collection := NewKeyedStreams[int](primitive, nil)
	input := Frame{}
	input.Put(deltaSymbol, 1)
	waitGroup := sync.WaitGroup{}

	for key := 0; key < 8; key++ {
		key := key
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for iteration := 0; iteration < 25; iteration++ {
				if _, err := collection.Step(key, input); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}

	waitGroup.Wait()

	for key := 0; key < 8; key++ {
		state, found := collection.Project(key)

		if !found || state.MustGet(totalSymbol) != 25 {
			t.Fatalf("key=%d total=%v found=%v; want 25", key, state.MustGet(totalSymbol), found)
		}
	}
}

func BenchmarkKeyedStreamsEstablishedKey(b *testing.B) {
	valueSymbol := MustIntern("keyed_stream_benchmark/value")
	primitive := func(state Frame, input Frame) (Frame, Frame, error) {
		nextState := state
		nextState.Put(valueSymbol, input.MustGet(valueSymbol))

		return nextState, nextState, nil
	}
	collection := NewKeyedStreams[string](primitive, nil)
	input := Frame{}
	input.Put(valueSymbol, 1)

	if _, err := collection.Step("STREAM/A", input); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := collection.Step("STREAM/A", input); err != nil {
			b.Fatal(err)
		}
	}
}
