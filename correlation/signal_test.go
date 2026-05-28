package correlation

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/adaptive"
)

func seedSymbolWindow(
	state *symbolState,
	basePrice float64,
	returns []float64,
	interval time.Duration,
	start time.Time,
) {
	price := basePrice
	state.window.Push(start, price)

	for index, logReturn := range returns {
		price *= math.Exp(logReturn)
		state.window.Push(start.Add(time.Duration(index+1)*interval), price)
	}

	state.last = price
}

func TestSignalMeasure(t *testing.T) {
	originalBar := config.System.CorrelationBarSeconds
	config.System.CorrelationBarSeconds = 10
	t.Cleanup(func() { config.System.CorrelationBarSeconds = originalBar })

	signal := &Signal{
		peak:       adaptive.NewPeak(),
		windowCap:  windowCap(),
		minSamples: config.System.MinCorrelationSamples,
	}

	returns := []float64{
		0.01, -0.005, 0.002, 0.004, -0.001, 0.003,
		0.002, -0.002, 0.001, 0.004, 0.003, 0.002,
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	interval := 10 * time.Second

	left := newSymbolState(asset.Pair{Wsname: "BTC/EUR"}, windowCap())
	right := newSymbolState(asset.Pair{Wsname: "ETH/EUR"}, windowCap())
	seedSymbolWindow(left, 100, returns, interval, start)
	seedSymbolWindow(right, 50, returns, interval, start)

	signal.symbols.Store("BTC/EUR", left)
	signal.symbols.Store("ETH/EUR", right)
	signal.requested.Store("BTC/EUR", struct{}{})
	signal.requested.Store("ETH/EUR", struct{}{})

	found := false

	for measurement := range signal.Measure() {
		found = true

		if measurement.Source != correlationSource || measurement.Confidence <= 0 ||
			measurement.Confidence > 1 {
			t.Fatalf("unexpected measurement: %+v", measurement)
		}
	}

	if !found {
		t.Fatal("expected correlation measurement")
	}
}

func TestSignalFeedbackScalesSymbol(t *testing.T) {
	signal := &Signal{}

	state := newSymbolState(asset.Pair{Wsname: "ETH/EUR"}, windowCap())
	state.last = 50
	signal.symbols.Store("ETH/EUR", state)

	before, ok := correlationMeasurement(state, 0.85)

	if !ok {
		t.Fatal("expected correlation measurement before feedback")
	}

	signal.Feedback(engine.PredictionFeedback{
		Source:          engine.PerspectiveSource(engine.PerspectiveCrossAsset),
		Sources:         []string{correlationSource},
		Symbol:          "ETH/EUR",
		PredictedReturn: 0.02,
		ActualReturn:    -0.01,
	})

	after, ok := correlationMeasurement(state, 0.85)

	if !ok {
		t.Fatal("expected correlation measurement after feedback")
	}

	if after.Confidence >= before.Confidence {
		t.Fatalf("expected feedback to lower confidence, before=%v after=%v", before.Confidence, after.Confidence)
	}
}

func TestSignalObserveTick(t *testing.T) {
	state := newSymbolState(asset.Pair{Wsname: "BTC/EUR"}, windowCap())

	state.observeTick(market.TickerRow{
		Symbol: "BTC/EUR",
		Last:   100,
		Bid:    99.9,
		Ask:    100.1,
	}, time.Now())

	if state.last != 100 {
		t.Fatalf("expected last price 100, got %v", state.last)
	}

	if len(state.window.Ordered()) != 1 {
		t.Fatalf("expected one price sample, got %d", len(state.window.Ordered()))
	}
}

func TestSymbolStateConcurrentObserveAndMeasure(t *testing.T) {
	state := newSymbolState(asset.Pair{Wsname: "BTC/EUR"}, windowCap())
	start := time.Unix(1_700_000_000, 0)
	var waiters sync.WaitGroup

	waiters.Go(func() {
		for index := range 128 {
			state.observeTick(market.TickerRow{
				Last: 100 + float64(index)*0.01,
				Bid:  99,
				Ask:  101,
			}, start.Add(time.Duration(index)*time.Second))
		}
	})
	waiters.Go(func() {
		for range 128 {
			correlationMeasurement(state, 0.8)
			state.snapshot()
		}
	})
	waiters.Go(func() {
		for range 128 {
			if err := state.applyFeedback(0.02, -0.01); err != nil {
				t.Errorf("feedback: %v", err)
			}
		}
	})

	waiters.Wait()
}
