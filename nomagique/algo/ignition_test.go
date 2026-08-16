package algo

import (
	"math"
	"reflect"
	"testing"

	"github.com/theapemachine/symm/nomagique/types"
)

const ignitionTestEpoch = 1_786_099_200.0

func TestIgnitionUsesOnlyInitialAndNext(t *testing.T) {
	structure := reflect.TypeOf(Ignition{})
	if structure.NumField() != 2 {
		t.Fatalf("Ignition has %d fields; want only initial and next", structure.NumField())
	}
	if structure.Field(0).Name != "initial" || structure.Field(1).Name != "next" {
		t.Fatalf("Ignition fields are %q and %q", structure.Field(0).Name, structure.Field(1).Name)
	}
}

func TestIgnitionReadinessIsCausal(t *testing.T) {
	process := newIgnitionForTest()

	first := measureIgnition(t, process, ignitionObservationForTest(
		"BTC/USD", 128, 20, 100, 99.5, 100.5, ignitionTestEpoch,
	))
	assertIgnitionNumber(t, first, "ready", 0, 0)
	assertIgnitionNumber(t, first, "rvol", 0, 0)
	assertIgnitionNumber(t, first, "ignition", 0, 0)
	for _, key := range []string{"strength", "buy/strength", "sell/strength"} {
		if _, found := first.Get(key); !found {
			t.Fatalf("provisional output key %q is missing", key)
		}
	}

	second := measureIgnition(t, process, ignitionObservationForTest(
		"BTC/USD", 128, 20, 101, 100.5, 101.5, ignitionTestEpoch+1,
	))
	assertIgnitionNumber(t, second, "ready", 0, 0)
	assertIgnitionNumber(t, second, "window/bars", 1, 0)

	third := measureIgnition(t, process, ignitionObservationForTest(
		"BTC/USD", 128, 20, 102, 101.5, 102.5, ignitionTestEpoch+2,
	))
	if ignitionNumber(third, "ready") != 1 {
		t.Fatalf("ready=%v; want 1", ignitionNumber(third, "ready"))
	}
	for _, key := range []string{"rvol", "precursor", "ignition"} {
		if ignitionNumber(third, key) <= 0 {
			t.Fatalf("%s=%v; want positive evidence", key, ignitionNumber(third, key))
		}
	}
}

func TestIgnitionRejectsInvalidObservation(t *testing.T) {
	process := newIgnitionForTest()
	invalid := ignitionObservationForTest(
		"BTC/USD", 0, 20, 100, 99.5, 100.5, ignitionTestEpoch,
	)
	process.Write(invalid)
	if process.Read().Error() == "" {
		t.Fatal("zero capacity should fail")
	}

	nonFinite := ignitionObservationForTest(
		"BTC/USD", 8, math.Inf(1), 100, 99.5, 100.5, ignitionTestEpoch,
	)
	process.Write(nonFinite)
	if process.Read().Error() == "" {
		t.Fatal("non-finite volume should fail")
	}
}

func TestIgnitionUsesEachStreamOwnScale(t *testing.T) {
	process := newIgnitionForTest()
	var smallOutput ignitionMap
	var largeOutput ignitionMap

	for index := 0; index < 8; index++ {
		at := ignitionTestEpoch + float64(index)
		smallOutput = measureIgnition(t, process, ignitionObservationForTest(
			"SMALL/USD", 128, 20,
			100+float64(index), 99.5+float64(index), 100.5+float64(index), at,
		))
		largeOutput = measureIgnition(t, process, ignitionObservationForTest(
			"LARGE/USD", 128, 200,
			1000+float64(index*10), 995+float64(index*10), 1005+float64(index*10), at,
		))
	}

	assertIgnitionNumber(t, smallOutput, "rvol", ignitionNumber(largeOutput, "rvol"), 1e-12)
	assertIgnitionNumber(t, smallOutput, "history/deltas/sample/0", 20, 0)
	assertIgnitionNumber(t, largeOutput, "history/deltas/sample/0", 200, 0)
}

func TestIgnitionRejectsTimeRegressionTransactionally(t *testing.T) {
	process := newIgnitionForTest()
	var committed ignitionMap
	for index := 0; index < 5; index++ {
		committed = measureIgnition(t, process, ignitionObservationForTest(
			"BTC/USD", 32, 20,
			100+float64(index), 99.5+float64(index), 100.5+float64(index),
			ignitionTestEpoch+float64(index),
		))
	}

	bars := ignitionNumber(committed, "window/bars")
	strength := ignitionNumber(committed, "strength")
	regressed := ignitionObservationForTest(
		"BTC/USD", 32, 20, 106, 105.5, 106.5, ignitionTestEpoch+2,
	)
	process.Write(regressed)
	failed := process.Read()
	if failed.Error() == "" {
		t.Fatal("regressed time should fail")
	}

	failedState := failed.Project().Read()
	retained, found := failedState.Value.Get("BTC/USD")
	if !found {
		t.Fatal("committed BTC/USD state is missing after rejection")
	}
	assertIgnitionNumber(t, retained, "window/bars", bars, 0)
	assertIgnitionNumber(t, retained, "strength", strength, 0)

	recovered := measureIgnition(t, process, ignitionObservationForTest(
		"BTC/USD", 32, 20, 106, 105.5, 106.5, ignitionTestEpoch+6,
	))
	if ignitionNumber(recovered, "window/bars") <= bars {
		t.Fatalf("bars=%v after recovery; want greater than %v", ignitionNumber(recovered, "window/bars"), bars)
	}
}

func TestIgnitionBoundsRetainedHistory(t *testing.T) {
	const capacity = 16
	process := newIgnitionForTest()
	var output ignitionMap
	for index := 0; index < 60; index++ {
		output = measureIgnition(t, process, ignitionObservationForTest(
			"BTC/USD", capacity, 20,
			100+float64(index), 99.5+float64(index), 100.5+float64(index),
			ignitionTestEpoch+float64(index),
		))
	}

	for _, name := range []string{"deltas", "rates", "returns", "precursors", "spreads"} {
		count := ignitionNumber(output, "history/"+name+"/count")
		if count > capacity {
			t.Fatalf("%s retained %v samples; capacity is %d", name, count, capacity)
		}
	}
}

func TestIgnitionReciprocalDirectionsAreSymmetric(t *testing.T) {
	bullish := newIgnitionForTest()
	bearish := newIgnitionForTest()
	prices := []float64{100, 101, 100, 102, 101, 104}
	var bull ignitionMap
	var bear ignitionMap

	for index, price := range prices {
		at := ignitionTestEpoch + float64(index)
		bull = measureIgnition(t, bullish, ignitionObservationForTest(
			"BULL/USD", 128, 20, price, price-0.5, price+0.5, at,
		))
		reciprocal := 1 / price
		spread := reciprocal / 100
		bear = measureIgnition(t, bearish, ignitionObservationForTest(
			"BEAR/USD", 128, 20, reciprocal,
			reciprocal-spread/2, reciprocal+spread/2, at,
		))
	}

	for _, key := range []string{"precursor", "ignition", "exhaustion"} {
		assertIgnitionNumber(
			t,
			bull,
			"buy/"+key,
			ignitionNumber(bear, "sell/"+key),
			1e-12,
		)
	}
	assertIgnitionNumber(t, bull, "rvol", ignitionNumber(bear, "rvol"), 1e-12)
}

func TestIgnitionRetainsMultipleKeyedStreams(t *testing.T) {
	process := newIgnitionForTest()
	measureIgnition(t, process, ignitionObservationForTest(
		"A/USD", 8, 20, 100, 99.5, 100.5, ignitionTestEpoch,
	))
	measureIgnition(t, process, ignitionObservationForTest(
		"B/USD", 8, 200, 1000, 995, 1005, ignitionTestEpoch,
	))
	measureIgnition(t, process, ignitionObservationForTest(
		"A/USD", 8, 20, 101, 100.5, 101.5, ignitionTestEpoch+1,
	))
	measureIgnition(t, process, ignitionObservationForTest(
		"B/USD", 8, 200, 1010, 1005, 1015, ignitionTestEpoch+1,
	))

	collection := process.Project().Read().Value
	a, hasA := collection.Get("A/USD")
	b, hasB := collection.Get("B/USD")
	if !hasA || !hasB {
		t.Fatalf("collection has A=%v B=%v; want both", hasA, hasB)
	}
	assertIgnitionNumber(t, a, "window/bars", 1, 0)
	assertIgnitionNumber(t, b, "window/bars", 1, 0)
	assertIgnitionNumber(t, a, "history/deltas/sample/0", 20, 0)
	assertIgnitionNumber(t, b, "history/deltas/sample/0", 200, 0)
}

func TestIgnitionClose(t *testing.T) {
	process := newIgnitionForTest()
	if err := process.Close(); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkIgnition(b *testing.B) {
	process := newIgnitionForTest()
	for index := 0; index < b.N; index++ {
		input := ignitionObservationForTest(
			"BTC/USD", 128, 20,
			100+float64(index%100), 99.5+float64(index%100), 100.5+float64(index%100),
			ignitionTestEpoch+float64(index),
		)
		process.Write(input)
		_ = process.Read()
	}
}

func newIgnitionForTest() *Ignition {
	return NewIgnition(types.NewInput[ignitionState]())
}

func ignitionObservationForTest(
	symbol string,
	capacity float64,
	volume float64,
	last float64,
	bid float64,
	ask float64,
	unixSec float64,
) types.Input[ignitionState] {
	mapping := types.NewMap[string, types.Value[float64]]()
	mapping.Put("capacity", types.NewValue(capacity))
	mapping.Put("volume", types.NewValue(volume))
	mapping.Put("last", types.NewValue(last))
	mapping.Put("bid", types.NewValue(bid))
	mapping.Put("ask", types.NewValue(ask))
	mapping.Put("unix_sec", types.NewValue(unixSec))
	mapping.Put("unix_nsec", types.NewValue(0.0))
	collection := types.NewMap[string, ignitionMap]()
	collection.Put(symbol, mapping)
	return types.NewInput(types.NewValue(types.NewPair(symbol, collection)))
}

func measureIgnition(t *testing.T, process *Ignition, input types.Input[ignitionState]) ignitionMap {
	t.Helper()
	process.Write(input)
	output := process.Read()
	if output.Error() != "" {
		t.Fatal(output.Error())
	}
	state := output.Project().Read()
	mapping, found := state.Value.Get(state.Key)
	if !found {
		t.Fatalf("active stream %q is missing", state.Key)
	}
	return mapping
}

func assertIgnitionNumber(
	t *testing.T,
	mapping ignitionMap,
	key string,
	want float64,
	tolerance float64,
) {
	t.Helper()
	got := ignitionNumber(mapping, key)
	if math.Abs(got-want) > tolerance {
		t.Fatalf("%s=%v; want %v (tolerance %v)", key, got, want, tolerance)
	}
}
