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
		score := lagScore{bars: 4, correlation: 0.5, reason: "leadlag_follower"}

		for range 128 {
			crossLag(anchor, lagger)
			lagMeasurement(anchor, lagger, 0.5, score, true)
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

	score := lagScore{bars: bars, correlation: correlation, reason: "leadlag_follower"}
	measurement, ok := lagMeasurement(anchor, lagger, 0.5, score, true)

	if !ok || measurement.Confidence <= 0 || measurement.Category == "" {
		t.Fatalf("expected lag measurement, got %+v ok=%v", measurement, ok)
	}
}

func TestCrossLagRejectsContemporaneousBeta(t *testing.T) {
	anchor := newSymbolState(asset.Pair{Wsname: "BTC/EUR"})
	lagger := newSymbolState(asset.Pair{Wsname: "ALT/EUR"})
	start := time.Unix(1_700_000_000, 0)
	returns := []float64{
		0.010, -0.007, 0.009, -0.006, 0.008, -0.005,
		0.007, -0.004, 0.006, -0.003, 0.005, -0.002,
		0.004, -0.003, 0.006, -0.004,
	}
	anchorPrice := 100.0
	laggerPrice := 50.0

	for index := range 128 {
		movement := returns[index%len(returns)]
		anchorPrice *= math.Exp(movement)
		laggerPrice *= math.Exp(movement)
		at := start.Add(time.Duration(index) * time.Second)

		anchor.observeTicker(0.08, anchorPrice, anchorPrice*0.999, anchorPrice*1.001, at)
		lagger.observeTicker(0.02, laggerPrice, laggerPrice*0.999, laggerPrice*1.001, at)
	}

	bars, correlation, ok := crossLag(anchor, lagger)

	if ok {
		t.Fatalf("expected beta path to be rejected, bars=%d correlation=%v", bars, correlation)
	}
}

func TestComputeLeadlagMarginUsesFloorAndRelativeScale(t *testing.T) {
	if got := computeLeadlagMargin(0.2); got != leadlagDominanceMarginAbs {
		t.Fatalf("expected absolute leadlag dominance margin floor, got %v", got)
	}

	expected := leadlagDominanceMarginRel * 0.9

	if got := computeLeadlagMargin(0.9); math.Abs(got-expected) > 1e-12 {
		t.Fatalf("expected relative leadlag dominance margin %v, got %v", expected, got)
	}
}

func TestLeadlagDominanceRejectsBestCorrAtCorr0PlusMargin(t *testing.T) {
	corr0 := 0.4
	bestCorr := corr0 + computeLeadlagMargin(corr0)
	ok := leadlagDominates(1, bestCorr, corr0)

	if ok {
		t.Fatalf("expected tie at bestCorr == corr0 + leadlag dominance margin to reject")
	}
}

func TestLeadlagDominanceRejectsNegativeCorr0WeakPositiveLag(t *testing.T) {
	corr0 := -0.8
	bestCorr := leadlagMinimumLagCorrelation
	ok := leadlagDominates(1, bestCorr, corr0)

	if ok {
		t.Fatalf("expected negative corr0 with weak positive bestCorr to reject")
	}
}

func TestLeadlagDominanceRejectsStrongCorrBelowAdaptiveMargin(t *testing.T) {
	corr0 := 0.9
	bestCorr := 1.0
	ok := leadlagDominates(1, bestCorr, corr0)

	if ok {
		t.Fatalf("expected strong corr0 with bestCorr below adaptive margin to reject")
	}
}
