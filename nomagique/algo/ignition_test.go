package algo

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique"
)

const ignitionTestEpoch = 1_786_099_200.0

func TestIgnitionReadinessIsCausal(t *testing.T) {
	stream := nomagique.NewStream(Ignition(), NewIgnitionState())
	first := measureIgnition(t, stream, ignitionObservationForTest(
		128,
		20,
		100,
		99.5,
		100.5,
		ignitionTestEpoch,
	))
	assertNumber(t, first, SymbolReady, 0)
	assertNumber(t, first, SymbolRVOL, 0)
	assertNumber(t, first, SymbolAlphaRVOL, 0)

	for _, symbol := range []nomagique.Symbol{
		SymbolAlphaRVOL,
		SymbolAlphaPrecursor,
		SymbolBetaRVOL,
	} {
		if !first.Has(symbol) {
			t.Fatalf("provisional output symbol %d is missing", symbol)
		}
	}

	second := measureIgnition(t, stream, ignitionObservationForTest(
		128,
		20,
		101,
		100.5,
		101.5,
		ignitionTestEpoch+1,
	))
	assertNumber(t, second, SymbolReady, 0)
	assertNumber(t, second, SymbolIgnitionBars, 1)

	third := measureIgnition(t, stream, ignitionObservationForTest(
		128,
		20,
		102,
		101.5,
		102.5,
		ignitionTestEpoch+2,
	))
	assertNumber(t, third, SymbolReady, 1)

	for _, symbol := range []nomagique.Symbol{
		SymbolRVOL,
		SymbolAlphaPrecursor,
	} {
		if third.MustGet(symbol) <= 0 {
			t.Fatalf("symbol %d=%v; want positive evidence", symbol, third.MustGet(symbol))
		}
	}
}

func TestIgnitionRejectsInvalidObservation(t *testing.T) {
	stream := nomagique.NewStream(Ignition(), NewIgnitionState())
	invalidCapacity := ignitionObservationForTest(
		0,
		20,
		100,
		99.5,
		100.5,
		ignitionTestEpoch,
	)

	if _, err := stream.Step(invalidCapacity); err == nil {
		t.Fatal("zero capacity should fail")
	}

	nonFiniteVolume := ignitionObservationForTest(
		8,
		math.Inf(1),
		100,
		99.5,
		100.5,
		ignitionTestEpoch,
	)

	if _, err := stream.Step(nonFiniteVolume); err == nil {
		t.Fatal("non-finite volume should fail")
	}
}

func TestIgnitionUsesEachStreamOwnScale(t *testing.T) {
	collection := nomagique.NewKeyedStreams[string](
		Ignition(),
		func(string) nomagique.Frame {
			return NewIgnitionState()
		},
	)
	var smallOutput nomagique.Frame
	var largeOutput nomagique.Frame

	for index := 0; index < 8; index++ {
		at := ignitionTestEpoch + float64(index)
		var err error
		smallOutput, err = collection.Step("SMALL/USD", ignitionObservationForTest(
			128,
			20,
			100+float64(index),
			99.5+float64(index),
			100.5+float64(index),
			at,
		))

		if err != nil {
			t.Fatal(err)
		}

		largeOutput, err = collection.Step("LARGE/USD", ignitionObservationForTest(
			128,
			200,
			1000+float64(index*10),
			995+float64(index*10),
			1005+float64(index*10),
			at,
		))

		if err != nil {
			t.Fatal(err)
		}
	}

	assertAlmostEqual(
		t,
		smallOutput.MustGet(SymbolRVOL),
		largeOutput.MustGet(SymbolRVOL),
		1e-12,
	)
	deltaSymbol, found := IgnitionHistorySample(HistoryDeltas, 0)

	if !found {
		t.Fatal("delta history symbol is missing")
	}

	assertNumber(t, smallOutput, deltaSymbol, 20)
	assertNumber(t, largeOutput, deltaSymbol, 200)
}

func TestIgnitionRejectsTimeRegressionTransactionally(t *testing.T) {
	stream := nomagique.NewStream(Ignition(), NewIgnitionState())
	var committed nomagique.Frame

	for index := 0; index < 5; index++ {
		committed = measureIgnition(t, stream, ignitionObservationForTest(
			32,
			20,
			100+float64(index),
			99.5+float64(index),
			100.5+float64(index),
			ignitionTestEpoch+float64(index),
		))
	}

	bars := committed.MustGet(SymbolIgnitionBars)
	rvol := committed.MustGet(SymbolRVOL)
	regressed := ignitionObservationForTest(
		32,
		20,
		106,
		105.5,
		106.5,
		ignitionTestEpoch+2,
	)

	if _, err := stream.Step(regressed); err == nil {
		t.Fatal("regressed time should fail")
	}

	retained := stream.Project()
	assertNumber(t, retained, SymbolIgnitionBars, bars)
	assertNumber(t, retained, SymbolRVOL, rvol)
	recovered := measureIgnition(t, stream, ignitionObservationForTest(
		32,
		20,
		106,
		105.5,
		106.5,
		ignitionTestEpoch+6,
	))

	if recovered.MustGet(SymbolIgnitionBars) <= bars {
		t.Fatalf(
			"bars=%v after recovery; want greater than %v",
			recovered.MustGet(SymbolIgnitionBars),
			bars,
		)
	}
}

func TestIgnitionKeepsReturnsAndPrecursorsSemanticallyDistinct(t *testing.T) {
	stream := nomagique.NewStream(Ignition(), NewIgnitionState())

	for index := 0; index < 3; index++ {
		measureIgnition(t, stream, ignitionObservationForTest(
			16,
			20,
			100,
			99.5,
			100.5,
			ignitionTestEpoch+float64(index),
		))
	}

	state := stream.Project()
	returns := IgnitionHistoryCount(state, HistoryReturns)
	precursors := IgnitionHistoryCount(state, HistoryPrecursors)

	if returns == 0 {
		t.Fatal("flat closed bars should retain zero returns")
	}

	if precursors != 0 {
		t.Fatalf("precursors=%d; flat moves should not be retained", precursors)
	}
}

func TestIgnitionBoundsRetainedHistory(t *testing.T) {
	const capacity = 16
	stream := nomagique.NewStream(Ignition(), NewIgnitionState())
	var output nomagique.Frame

	for index := 0; index < 60; index++ {
		output = measureIgnition(t, stream, ignitionObservationForTest(
			capacity,
			20,
			100+float64(index),
			99.5+float64(index),
			100.5+float64(index),
			ignitionTestEpoch+float64(index),
		))
	}

	for _, history := range []string{
		HistoryDeltas,
		HistoryRates,
		HistoryReturns,
		HistoryPrecursors,
	} {
		count := IgnitionHistoryCount(output, history)

		if count > capacity {
			t.Fatalf("%s retained %d samples; capacity is %d", history, count, capacity)
		}
	}
}

func TestIgnitionReciprocalDirectionsAreSymmetric(t *testing.T) {
	bullish := nomagique.NewStream(Ignition(), NewIgnitionState())
	bearish := nomagique.NewStream(Ignition(), NewIgnitionState())
	prices := []float64{100, 101, 100, 102, 101, 104}
	var bull nomagique.Frame
	var bear nomagique.Frame

	for index, price := range prices {
		at := ignitionTestEpoch + float64(index)
		bull = measureIgnition(t, bullish, ignitionObservationForTest(
			128,
			20,
			price,
			price-0.5,
			price+0.5,
			at,
		))
		reciprocal := 1 / price
		spread := reciprocal / 100
		bear = measureIgnition(t, bearish, ignitionObservationForTest(
			128,
			20,
			reciprocal,
			reciprocal-spread/2,
			reciprocal+spread/2,
			at,
		))
	}

	for _, pair := range []struct {
		bull nomagique.Symbol
		bear nomagique.Symbol
	}{
		{bull: SymbolAlphaPrecursor, bear: SymbolBetaPrecursor},
		{bull: SymbolAlphaExhaustion, bear: SymbolBetaExhaustion},
	} {
		assertAlmostEqual(t, bull.MustGet(pair.bull), bear.MustGet(pair.bear), 1e-12)
	}

	assertAlmostEqual(t, bull.MustGet(SymbolRVOL), bear.MustGet(SymbolRVOL), 1e-12)
}

func TestIgnitionRetainsMultipleKeyedStreams(t *testing.T) {
	collection := nomagique.NewKeyedStreams[string](Ignition(), nil)
	observations := []struct {
		key    string
		volume float64
		last   float64
		bid    float64
		ask    float64
		at     float64
	}{
		{key: "A/USD", volume: 20, last: 100, bid: 99.5, ask: 100.5, at: ignitionTestEpoch},
		{key: "B/USD", volume: 200, last: 1000, bid: 995, ask: 1005, at: ignitionTestEpoch},
		{key: "A/USD", volume: 20, last: 101, bid: 100.5, ask: 101.5, at: ignitionTestEpoch + 1},
		{key: "B/USD", volume: 200, last: 1010, bid: 1005, ask: 1015, at: ignitionTestEpoch + 1},
	}

	for _, observation := range observations {
		_, err := collection.Step(observation.key, ignitionObservationForTest(
			8,
			observation.volume,
			observation.last,
			observation.bid,
			observation.ask,
			observation.at,
		))

		if err != nil {
			t.Fatal(err)
		}
	}

	first, hasFirst := collection.Project("A/USD")
	second, hasSecond := collection.Project("B/USD")

	if !hasFirst || !hasSecond {
		t.Fatalf("collection has A=%v B=%v; want both", hasFirst, hasSecond)
	}

	assertNumber(t, first, SymbolIgnitionBars, 1)
	assertNumber(t, second, SymbolIgnitionBars, 1)
	deltaSymbol, found := IgnitionHistorySample(HistoryDeltas, 0)

	if !found {
		t.Fatal("delta history symbol is missing")
	}

	assertNumber(t, first, deltaSymbol, 20)
	assertNumber(t, second, deltaSymbol, 200)
}

func TestAlgorithmSymbolsFitFrame(t *testing.T) {
	if nomagique.RegisteredSymbols() > nomagique.MaxSlots {
		t.Fatalf(
			"registered symbols=%d; Frame capacity is %d",
			nomagique.RegisteredSymbols(),
			nomagique.MaxSlots,
		)
	}
}

func BenchmarkIgnition(b *testing.B) {
	stream := nomagique.NewStream(Ignition(), NewIgnitionState())

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		input := ignitionObservationForTest(
			128,
			20,
			100+float64(iteration%100),
			99.5+float64(iteration%100),
			100.5+float64(iteration%100),
			ignitionTestEpoch+float64(iteration),
		)

		if _, err := stream.Step(input); err != nil {
			b.Fatal(err)
		}
	}
}

func ignitionObservationForTest(
	capacity float64,
	volume float64,
	last float64,
	bid float64,
	ask float64,
	unixSec float64,
) nomagique.Frame {
	input := nomagique.Frame{}
	input.Put(SymbolCapacity, capacity)
	input.Put(SymbolVolume, volume)
	input.Put(SymbolLast, last)
	input.Put(SymbolBid, bid)
	input.Put(SymbolAsk, ask)
	input.Put(SymbolUnixSec, unixSec)
	input.Put(SymbolUnixNsec, 0)

	return input
}

func measureIgnition(
	t *testing.T,
	stream *nomagique.Stream,
	input nomagique.Frame,
) nomagique.Frame {
	t.Helper()
	output, err := stream.Step(input)

	if err != nil {
		t.Fatal(err)
	}

	return output
}

func assertAlmostEqual(t *testing.T, got float64, want float64, tolerance float64) {
	t.Helper()

	if math.Abs(got-want) > tolerance {
		t.Fatalf("got=%v; want %v (tolerance %v)", got, want, tolerance)
	}
}
