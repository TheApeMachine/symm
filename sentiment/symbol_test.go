package sentiment

import (
	"sync"
	"testing"

	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
)

func TestSymbolStateCalibratedConfidence(t *testing.T) {
	state := newSymbolState(asset.Pair{Wsname: "A/EUR"})
	before := state.calibratedConfidence(0.5)

	if _, err := state.forecastLearner().Next(0, 0.02, -0.02); err != nil {
		t.Fatalf("forecast feedback: %v", err)
	}

	after := state.calibratedConfidence(0.5)

	if after >= before {
		t.Fatalf("expected losing feedback to lower confidence, before=%v after=%v", before, after)
	}
}

func TestSymbolStateConcurrentObserveAndCalibrate(t *testing.T) {
	state := newSymbolState(asset.Pair{Wsname: "A/EUR"})
	var waiters sync.WaitGroup

	waiters.Go(func() {
		for index := range 128 {
			state.observeTicker(market.TickerRow{
				Last:      10 + float64(index)*0.01,
				Bid:       9.9,
				Ask:       10.1,
				ChangePct: 0.2,
			})
		}
	})
	waiters.Go(func() {
		for range 128 {
			state.calibratedConfidence(0.5)
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
