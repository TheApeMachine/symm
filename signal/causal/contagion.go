package causal

import (
	"math"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/ring"
)

// contagionSymbolCap bounds how many symbols enter the pairwise correlation sweep, keeping the
// universe-level computation O(cap^2) regardless of how many symbols are subscribed.
const contagionSymbolCap = 16

func contagionWindow() int {
	window := viper.GetViper().GetInt("signals.causal.contagion_window")

	if window > 0 {
		return window
	}

	return 128
}

func contagionMinSamples() int {
	minSamples := viper.GetViper().GetInt("signals.causal.contagion_min_samples")

	if minSamples > 0 {
		return minSamples
	}

	return 16
}

type contagionTier struct {
	fast   float64
	medium float64
	slow   float64
}

/*
contagion measures cross-asset coupling across the subscribed universe as the median absolute
Hayashi-Yoshida correlation over symbol pairs. Crypto venues are normally correlated, so it is
the spike toward one — every asset moving as a single block during a liquidation cascade — that
flips the structural causal model into its panic regime. Returns zero when too few symbols carry
enough return history to form a stable estimate.

Adaptive windowing compares fast, medium, and slow tiers: when the fast tier diverges sharply
above the slow baseline the estimator surfaces the fast reading immediately; otherwise the
medium tier anchors the estimate so momentary blips do not false-trigger panic.
*/
func (signal *Signal) contagion() float64 {
	if signal.contagionSpread.Cap() == 0 {
		signal.contagionSpread = ring.NewFloatRing(64)
	}

	tier := signal.contagionTiers()

	if tier.medium <= 0 && tier.fast <= 0 && tier.slow <= 0 {
		return 0
	}

	return adaptiveContagion(tier, &signal.contagionSpread)
}

func (signal *Signal) contagionTiers() contagionTier {
	fastSnapshots := make([]*hyReturns, 0, contagionSymbolCap)
	mediumSnapshots := make([]*hyReturns, 0, contagionSymbolCap)
	slowSnapshots := make([]*hyReturns, 0, contagionSymbolCap)
	minSamples := contagionMinSamples()

	signal.symbols.Range(func(key, value any) bool {
		state := value.(*CausalSymbol)
		windows := state.HYWindowSnapshot()

		if windows == nil {
			return true
		}

		if snapshot := windows.fast; snapshot != nil && snapshot.len() >= minSamples {
			fastSnapshots = append(fastSnapshots, snapshot)
		}

		if snapshot := windows.medium; snapshot != nil && snapshot.len() >= minSamples {
			mediumSnapshots = append(mediumSnapshots, snapshot)
		}

		if snapshot := windows.slow; snapshot != nil && snapshot.len() >= minSamples {
			slowSnapshots = append(slowSnapshots, snapshot)
		}

		return len(slowSnapshots) < contagionSymbolCap
	})

	return contagionTier{
		fast:   medianPairwiseCorrelation(fastSnapshots),
		medium: medianPairwiseCorrelation(mediumSnapshots),
		slow:   medianPairwiseCorrelation(slowSnapshots),
	}
}

func medianPairwiseCorrelation(snapshots []*hyReturns) float64 {
	if len(snapshots) < 2 {
		return 0
	}

	correlations := make([]float64, 0, len(snapshots)*(len(snapshots)-1)/2)

	for left := 0; left < len(snapshots); left++ {
		for right := left + 1; right < len(snapshots); right++ {
			if correlation, ok := hayashiYoshidaCorrelation(snapshots[left], snapshots[right]); ok {
				correlations = append(correlations, math.Abs(correlation))
			}
		}
	}

	if len(correlations) == 0 {
		return 0
	}

	return numeric.PercentileSorted(numeric.CopySorted(correlations), 0.5)
}

func adaptiveContagion(tier contagionTier, spreadHistory *ring.FloatRing) float64 {
	if tier.fast <= 0 && tier.medium <= 0 {
		return tier.slow
	}

	if tier.slow <= 0 {
		if tier.medium > 0 {
			return tier.medium
		}

		return tier.fast
	}

	spread := tier.fast - tier.slow

	if spreadHistory != nil {
		spreadHistory.Push(spread)
	}

	threshold := contagionAdaptiveThreshold(spreadHistory, tier.slow)

	if spread > threshold {
		return tier.fast
	}

	if tier.medium > 0 {
		return tier.medium
	}

	return tier.slow
}

func contagionAdaptiveThreshold(spreadHistory *ring.FloatRing, slowBaseline float64) float64 {
	sigma := contagionAdaptiveSigma()
	floor := slowBaseline * 0.05

	if spreadHistory == nil || spreadHistory.Len() < 4 {
		return math.Max(floor, 0.15)
	}

	mean, stddev := spreadHistory.MeanStdDev()

	if stddev <= 0 {
		return math.Max(floor, mean)
	}

	return math.Max(floor, mean+sigma*stddev)
}
