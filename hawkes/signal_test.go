package hawkes

import (
	"testing"
	"time"
)

func burstBuyEvents(start time.Time, count int, gap time.Duration) []time.Time {
	events := make([]time.Time, count)

	for index := range events {
		events[index] = start.Add(time.Duration(index) * gap)
	}

	return events
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

func TestConfidenceSpikeUsesOwnFence(t *testing.T) {
	trackStore := NewTrackStore()

	for index := 0; index < minFitEvents; index++ {
		trackStore.bySymbol["PUMP/EUR"] = trackStore.ensure("PUMP/EUR")
		trackStore.bySymbol["PUMP/EUR"].confidenceHistory = append(
			trackStore.bySymbol["PUMP/EUR"].confidenceHistory,
			1.2,
		)
	}

	if _, ok := trackStore.ConfidenceSpike("PUMP/EUR", 2.5); !ok {
		t.Fatal("expected confidence above own fence to fire")
	}

	if _, ok := trackStore.ConfidenceSpike("PUMP/EUR", 1.1); ok {
		t.Fatal("expected confidence below fence to be rejected")
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
