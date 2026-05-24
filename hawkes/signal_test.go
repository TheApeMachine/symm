package hawkes

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken/market"
)

const testFixtureWindow = 5 * time.Minute

func burstEvents(start time.Time, count int, gap time.Duration) []time.Time {
	events := make([]time.Time, count)

	for index := range events {
		events[index] = start.Add(time.Duration(index) * gap)
	}

	return events
}

func sparseSellEvents(start time.Time, count int) []time.Time {
	sells := make([]time.Time, count)

	for index := range sells {
		sells[index] = start.Add(time.Duration(index+1) * time.Second)
	}

	return sells
}

func balancedBurstEvents(
	start time.Time,
	buyCount, sellCount int,
	buyGap time.Duration,
) ([]time.Time, []time.Time) {
	return burstEvents(start, buyCount, buyGap), sparseSellEvents(start.Add(-time.Second), sellCount)
}

func TestSplitSideEventsKeepsWindowTicks(t *testing.T) {
	now := time.Unix(1000, 0)
	windowStart := now.Add(-testFixtureWindow)

	ticks := []market.TradeTick{
		{Side: "buy", Timestamp: windowStart.Add(-time.Minute)},
		{Side: "buy", Timestamp: windowStart.Add(time.Minute)},
		{Side: "sell", Timestamp: now.Add(-time.Second)},
		{Side: "buy", Timestamp: now.Add(time.Minute)},
	}

	buyTimes, sellTimes := splitSideEvents(ticks, windowStart, now)

	if len(buyTimes) != 1 {
		t.Fatalf("expected one buy event in window, got %d", len(buyTimes))
	}

	if len(sellTimes) != 1 {
		t.Fatalf("expected one sell event in window, got %d", len(sellTimes))
	}

	if buyTimes[0] != ticks[1].Timestamp {
		t.Fatalf("expected in-window buy tick, got %v", buyTimes[0])
	}
}

func TestMergeMarkedEventsSortsChronologically(t *testing.T) {
	start := time.Unix(0, 0)
	buyEvents := []time.Time{start.Add(3 * time.Second), start}
	sellEvents := []time.Time{start.Add(2 * time.Second)}

	marked := mergeMarkedEvents(buyEvents, sellEvents)

	if len(marked) != 3 {
		t.Fatalf("expected three marked events, got %d", len(marked))
	}

	if !marked[0].at.Equal(start) {
		t.Fatalf("expected first event at start, got %v", marked[0].at)
	}

	if marked[1].side != sideSell {
		t.Fatal("expected second event to be sell")
	}

	if marked[2].side != sideBuy {
		t.Fatal("expected third event to be buy")
	}
}

func TestNewFitContextDerivesBoundsFromGaps(t *testing.T) {
	start := time.Unix(0, 0)
	buyEvents, sellEvents := balancedBurstEvents(start, 16, 6, 50*time.Millisecond)
	horizon := buyEvents[len(buyEvents)-1].Add(10 * time.Millisecond)
	context, ok := newFitContext(buyEvents, sellEvents, horizon)

	if !ok {
		t.Fatal("expected fit context")
	}

	if context.MinFitEvents <= 0 || context.MinPerSide <= 0 {
		t.Fatalf("expected positive fit thresholds, min=%d side=%d", context.MinFitEvents, context.MinPerSide)
	}

	if len(context.BetaCandidates) < 3 {
		t.Fatalf("expected beta candidates from event count, got %d", len(context.BetaCandidates))
	}

	if context.BranchCeiling >= criticalBranch {
		t.Fatalf("expected subcritical branch ceiling, got %v", context.BranchCeiling)
	}

	if context.TradeWindow <= 0 {
		t.Fatal("expected positive trade window")
	}
}

func TestFitBivariateRecoversExcitation(t *testing.T) {
	start := time.Unix(0, 0)
	buyEvents, sellEvents := balancedBurstEvents(start, 16, 6, 50*time.Millisecond)
	horizon := buyEvents[len(buyEvents)-1].Add(10 * time.Millisecond)
	fit := fitBivariate(buyEvents, sellEvents, horizon)

	if fit.MuBuy <= 0 || fit.Beta <= 0 {
		t.Fatalf("expected fitted parameters, muBuy=%v beta=%v", fit.MuBuy, fit.Beta)
	}

	if fit.BuyIntensity <= fit.MuBuy {
		t.Fatalf(
			"expected self-exciting buy intensity above baseline, muBuy=%v intensity=%v",
			fit.MuBuy, fit.BuyIntensity,
		)
	}

	if fit.SpectralRadius <= 0 || fit.SpectralRadius >= criticalBranch {
		t.Fatalf("expected subcritical spectral radius, got %v", fit.SpectralRadius)
	}
}

func TestFitBivariateCapturesCrossExcitation(t *testing.T) {
	start := time.Unix(0, 0)
	sellEvents := burstEvents(start, 16, 55*time.Millisecond)
	buyEvents := make([]time.Time, 6)

	for index := range buyEvents {
		buyEvents[index] = sellEvents[index].Add(20 * time.Millisecond)
	}

	horizon := sellEvents[len(sellEvents)-1].Add(100 * time.Millisecond)
	fit := fitBivariate(buyEvents, sellEvents, horizon)

	if fit.MuBuy <= 0 {
		t.Fatal("expected bivariate fit for sell-then-buy cascade")
	}

	fittedLL := bivariateLogLikelihood(
		buyEvents, sellEvents, horizon,
		fit.MuBuy, fit.MuSell,
		fit.AlphaBB, fit.AlphaBS, fit.AlphaSB, fit.AlphaSS, fit.Beta,
	)
	noCrossLL := bivariateLogLikelihood(
		buyEvents, sellEvents, horizon,
		fit.MuBuy, fit.MuSell,
		fit.AlphaBB, 0, 0, fit.AlphaSS, fit.Beta,
	)

	if fittedLL <= noCrossLL {
		t.Fatalf(
			"expected cross-kernel likelihood above diagonal-only, fitted=%v noCross=%v",
			fittedLL, noCrossLL,
		)
	}

	if fit.AlphaBS <= 0 && fit.AlphaSB <= 0 {
		t.Fatalf(
			"expected non-zero cross excitation, alphaBS=%v alphaSB=%v",
			fit.AlphaBS, fit.AlphaSB,
		)
	}
}

func TestBuySellAsymmetryRequiresBuyDominance(t *testing.T) {
	fit := BivariateFit{BuyIntensity: 3, SellIntensity: 1, MuBuy: 1, SpectralRadius: 0.4}

	if buySellAsymmetry(fit) <= 0 {
		t.Fatal("expected positive asymmetry when buy intensity dominates")
	}

	fit.SellIntensity = 4

	if buySellAsymmetry(fit) != 0 {
		t.Fatal("expected zero asymmetry when sell intensity dominates")
	}
}

func TestExcitationConfidenceRejectsCriticalBranching(t *testing.T) {
	fit := BivariateFit{
		BuyIntensity:   4,
		MuBuy:          1,
		SpectralRadius: 1.05,
	}

	if excitationConfidence(fit, 0.5, 1, false) != 0 {
		t.Fatal("expected zero confidence at critical spectral radius")
	}
}

func TestExcitationConfidenceUsesBaselineFence(t *testing.T) {
	fit := BivariateFit{
		BuyIntensity:   2,
		MuBuy:          1,
		SpectralRadius: 0.4,
	}

	if excitationConfidence(fit, 0.5, 3, false) != 0 {
		t.Fatal("expected zero confidence below symbol baseline fence")
	}
}

func TestSpectralRadiusSubcritical(t *testing.T) {
	radius := spectralRadius(0.2, 0.05, 0.05, 0.15, 1)

	if radius <= 0 || radius >= criticalBranch {
		t.Fatalf("expected subcritical spectral radius, got %v", radius)
	}
}

func TestRecordScoreStoresConfidence(t *testing.T) {
	trackStore := NewTrackStore()
	minHistory := confidenceHistoryCap(bivariateParamCount * 2)

	for index := 0; index < minHistory; index++ {
		trackStore.bySymbol["PUMP/EUR"] = trackStore.track("PUMP/EUR")
		trackStore.bySymbol["PUMP/EUR"].confidenceHistory = append(
			trackStore.bySymbol["PUMP/EUR"].confidenceHistory,
			1.2,
		)
	}

	if score := trackStore.RecordScore("PUMP/EUR", 2.5); score <= 0 || score > 1 {
		t.Fatalf("expected unit-scale score in (0,1], got %v", score)
	}

	if score := trackStore.RecordScore("PUMP/EUR", 1.1); score <= 0 || score > 1 {
		t.Fatalf("expected unit-scale score in (0,1], got %v", score)
	}
}

func TestRecordScoreNormalizesFirstSampleToOne(t *testing.T) {
	trackStore := NewTrackStore()

	score := trackStore.RecordScore("PUMP/EUR", 3.2)

	if score != 1 {
		t.Fatalf("expected first sample normalized to 1, got %v", score)
	}
}

func TestRecordScoreScalesAgainstSymbolFence(t *testing.T) {
	trackStore := NewTrackStore()
	track := trackStore.track("PUMP/EUR")
	track.confidenceHistory = []float64{1, 1.2, 1.4, 1.6}
	fence := confidenceFence(track.confidenceHistory)
	score := track.normalizedConfidence(fence / 2)

	if score <= 0 || score >= 1 {
		t.Fatalf("expected mid-fence score in (0,1), got %v", score)
	}
}

func TestRecordScoreRejectsNonPositive(t *testing.T) {
	trackStore := NewTrackStore()

	if score := trackStore.RecordScore("PUMP/EUR", 0); score != 0 {
		t.Fatalf("expected zero score, got %v", score)
	}
}

func TestFitBivariateWarmStartMatchesFullSearch(t *testing.T) {
	start := time.Unix(0, 0)
	buyEvents, sellEvents := balancedBurstEvents(start, 16, 6, 50*time.Millisecond)
	horizon := buyEvents[len(buyEvents)-1].Add(10 * time.Millisecond)
	context, ok := newFitContext(buyEvents, sellEvents, horizon)

	if !ok {
		t.Fatal("expected fit context")
	}

	full := scanBivariateFullGrid(
		buyEvents, sellEvents, horizon, context,
		float64(len(buyEvents))/context.SpanSec,
		float64(len(sellEvents))/context.SpanSec,
	)
	warm := fitBivariateWithPrior(buyEvents, sellEvents, horizon, full)

	if warm.MuBuy <= 0 || warm.Beta <= 0 {
		t.Fatalf("expected warm-started fit, muBuy=%v beta=%v", warm.MuBuy, warm.Beta)
	}

	if warm.BuyIntensity <= warm.MuBuy {
		t.Fatal("expected warm-started buy intensity above baseline")
	}
}

func TestFitBivariateWarmStartUsesPrior(t *testing.T) {
	start := time.Unix(0, 0)
	buyEvents, sellEvents := balancedBurstEvents(start, 16, 6, 50*time.Millisecond)
	horizon := buyEvents[len(buyEvents)-1].Add(10 * time.Millisecond)
	prior := fitBivariate(buyEvents, sellEvents, horizon)
	second := fitBivariateWithPrior(buyEvents, sellEvents, horizon, prior)

	if second.MuBuy <= 0 {
		t.Fatal("expected second warm fit")
	}
}

func BenchmarkFitBivariateWarmStart(b *testing.B) {
	start := time.Unix(0, 0)
	buyEvents, sellEvents := balancedBurstEvents(start, 32, 12, 25*time.Millisecond)
	horizon := buyEvents[len(buyEvents)-1].Add(time.Millisecond)
	prior := fitBivariate(buyEvents, sellEvents, horizon)

	b.ReportAllocs()

	for b.Loop() {
		if fit := fitBivariateWithPrior(buyEvents, sellEvents, horizon, prior); fit.MuBuy <= 0 {
			b.Fatal("expected fit")
		}
	}
}

func BenchmarkFitBivariate(b *testing.B) {
	start := time.Unix(0, 0)
	buyEvents, sellEvents := balancedBurstEvents(start, 32, 12, 25*time.Millisecond)
	horizon := buyEvents[len(buyEvents)-1].Add(time.Millisecond)

	b.ReportAllocs()

	for b.Loop() {
		if fit := fitBivariate(buyEvents, sellEvents, horizon); fit.MuBuy <= 0 {
			b.Fatal("expected fit")
		}
	}
}
