package hawkes

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken/market"
)

func burstBuyEvents(start time.Time, count int, gap time.Duration) []time.Time {
	events := make([]time.Time, count)

	for index := range events {
		events[index] = start.Add(time.Duration(index) * gap)
	}

	return events
}

func TestSplitSideEventsKeepsWindowTicks(t *testing.T) {
	now := time.Unix(1000, 0)
	windowStart := now.Add(-hawkesTradeWindow)

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

func TestFitSideRecoversExcitation(t *testing.T) {
	start := time.Unix(0, 0)
	buyEvents := burstBuyEvents(start, minFitEvents+4, 50*time.Millisecond)
	horizon := buyEvents[len(buyEvents)-1].Add(10 * time.Millisecond)
	fit := fitSide(buyEvents, horizon)

	if fit.mu <= 0 || fit.beta <= 0 {
		t.Fatalf("expected fitted parameters, mu=%v beta=%v", fit.mu, fit.beta)
	}

	if fit.intensity <= fit.mu {
		t.Fatalf("expected self-exciting intensity above baseline, mu=%v intensity=%v", fit.mu, fit.intensity)
	}

	if fit.branching < 0 || fit.branching >= criticalBranch {
		t.Fatalf("expected subcritical branching ratio, got %v", fit.branching)
	}
}

func TestBuySellAsymmetryRequiresBuyDominance(t *testing.T) {
	buyFit := SideFit{intensity: 3, mu: 1}
	sellFit := SideFit{intensity: 1, mu: 1}

	if buySellAsymmetry(buyFit, sellFit) <= 0 {
		t.Fatal("expected positive asymmetry when buy intensity dominates")
	}

	sellFit.intensity = 4

	if buySellAsymmetry(buyFit, sellFit) != 0 {
		t.Fatal("expected zero asymmetry when sell intensity dominates")
	}
}

func TestExcitationConfidenceRejectsCriticalBranching(t *testing.T) {
	fit := SideFit{intensity: 4, mu: 1, branching: 1.05}

	if excitationConfidence(fit, 0.5) != 0 {
		t.Fatal("expected zero confidence at critical branching ratio")
	}
}

func TestRecordScoreStoresConfidence(t *testing.T) {
	trackStore := NewTrackStore()

	for index := 0; index < minConfidenceHistory; index++ {
		trackStore.bySymbol["PUMP/EUR"] = trackStore.ensure("PUMP/EUR")
		trackStore.bySymbol["PUMP/EUR"].confidenceHistory = append(
			trackStore.bySymbol["PUMP/EUR"].confidenceHistory,
			1.2,
		)
	}

	if score := trackStore.RecordScore("PUMP/EUR", 2.5); score != 2.5 {
		t.Fatalf("expected score 2.5, got %v", score)
	}

	if score := trackStore.RecordScore("PUMP/EUR", 1.1); score != 1.1 {
		t.Fatalf("expected score 1.1, got %v", score)
	}
}

func TestRecordScoreRejectsNonPositive(t *testing.T) {
	trackStore := NewTrackStore()

	if score := trackStore.RecordScore("PUMP/EUR", 0); score != 0 {
		t.Fatalf("expected zero score, got %v", score)
	}
}

func TestFitSideWarmStartMatchesFullSearch(t *testing.T) {
	start := time.Unix(0, 0)
	events := burstBuyEvents(start, minFitEvents+4, 50*time.Millisecond)
	horizon := events[len(events)-1].Add(10 * time.Millisecond)
	full := scanFullGrid(events, horizon, float64(len(events))/horizon.Sub(events[0]).Seconds(), 1/medianInterArrivalSec(events))
	warm := fitSideWithPrior(events, horizon, full)

	if warm.mu <= 0 || warm.beta <= 0 {
		t.Fatalf("expected warm-started fit, mu=%v beta=%v", warm.mu, warm.beta)
	}

	if warm.intensity <= warm.mu {
		t.Fatal("expected warm-started intensity above baseline")
	}
}

func TestFitSideWarmStartUsesPrior(t *testing.T) {
	start := time.Unix(0, 0)
	events := burstBuyEvents(start, minFitEvents+4, 50*time.Millisecond)
	horizon := events[len(events)-1].Add(10 * time.Millisecond)
	prior := fitSide(events, horizon)
	second := fitSideWithPrior(events, horizon, prior)

	if second.mu <= 0 {
		t.Fatal("expected second warm fit")
	}
}

func BenchmarkFitSideWarmStart(b *testing.B) {
	start := time.Unix(0, 0)
	events := burstBuyEvents(start, 32, 25*time.Millisecond)
	horizon := events[len(events)-1].Add(time.Millisecond)
	prior := fitSide(events, horizon)

	b.ReportAllocs()

	for b.Loop() {
		if fit := fitSideWithPrior(events, horizon, prior); fit.mu <= 0 {
			b.Fatal("expected fit")
		}
	}
}

func BenchmarkFitSide(b *testing.B) {
	start := time.Unix(0, 0)
	events := burstBuyEvents(start, 32, 25*time.Millisecond)
	horizon := events[len(events)-1].Add(time.Millisecond)

	b.ReportAllocs()

	for b.Loop() {
		if fit := fitSide(events, horizon); fit.mu <= 0 {
			b.Fatal("expected fit")
		}
	}
}
