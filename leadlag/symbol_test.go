package leadlag

import (
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
	var waiters sync.WaitGroup

	waiters.Go(func() {
		for index := range 128 {
			anchor.observeTicker(
				0.08,
				100+float64(index)*0.1,
				99,
				101,
				start.Add(time.Duration(index)*time.Second),
			)
		}
	})
	waiters.Go(func() {
		for index := range 128 {
			lagger.observeTicker(
				0.02,
				50+float64(index)*0.05,
				49,
				51,
				start.Add(time.Duration(index)*time.Second+500*time.Millisecond),
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
}
