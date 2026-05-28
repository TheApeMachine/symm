package leadlag

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken/asset"
)

func TestSymbolStateForecastScale(t *testing.T) {
	state := newSymbolState(asset.Pair{Wsname: "ALT/EUR"})
	before := state.forecastScale()

	if _, err := state.forecastLearner().Next(0, 0.02, -0.02); err != nil {
		t.Fatalf("forecast feedback: %v", err)
	}

	after := state.forecastScale()

	if after >= before {
		t.Fatalf("expected losing feedback to lower scale, before=%v after=%v", before, after)
	}
}

func TestSymbolStateConcurrentObserveAndLag(t *testing.T) {
	anchor := newSymbolState(asset.Pair{Wsname: "BTC/EUR"})
	lagger := newSymbolState(asset.Pair{Wsname: "ALT/EUR"})
	start := time.Unix(1_700_000_000, 0)
	returns := []float64{
		0.010, -0.007, 0.009, -0.006, 0.008, -0.005,
		0.007, -0.004, 0.006, -0.003, 0.005, -0.002,
		0.004, -0.003, 0.006, -0.004,
	}
	var waiters sync.WaitGroup

	waiters.Go(func() {
		price := 100.0

		for index := range 128 {
			price *= math.Exp(returns[index%len(returns)])
			anchor.observeTicker(
				0.08,
				price,
				price*0.999,
				price*1.001,
				start.Add(time.Duration(index)*time.Second),
			)
		}
	})
	waiters.Go(func() {
		price := 50.0

		for index := range 128 {
			price *= math.Exp(returns[index%len(returns)])
			lagger.observeTicker(
				0.02,
				price,
				price*0.999,
				price*1.001,
				start.Add(time.Duration(index)*time.Second+10*time.Second),
			)
		}
	})
	waiters.Go(func() {
		for range 128 {
			crossLag(anchor, lagger)
			lagMeasurement(anchor, lagger, 0.5, 0.5)
		}
	})

	waiters.Wait()

	if len(anchor.priceSamples()) != 128 || len(lagger.priceSamples()) != 128 {
		t.Fatalf("expected 128 samples for both symbols")
	}

	bars, correlation, ok := crossLag(anchor, lagger)

	if !ok || bars <= 0 || bars > maxLagBars || correlation <= 0 || correlation > 1 {
		t.Fatalf("unexpected cross lag: bars=%d correlation=%v ok=%v", bars, correlation, ok)
	}

	measurement, ok := lagMeasurement(anchor, lagger, 0.5, correlation)

	if !ok || measurement.Confidence <= 0 {
		t.Fatalf("expected lag measurement, got %+v ok=%v", measurement, ok)
	}
}
