package causal

import (
	"math"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/numeric"
)

// contagionSymbolCap bounds how many symbols enter the pairwise correlation sweep, keeping the
// universe-level computation O(cap^2) regardless of how many symbols are subscribed.
const contagionSymbolCap = 16

func contagionWindow() int {
	if config.System.CausalContagionWindow > 0 {
		return config.System.CausalContagionWindow
	}

	return 128
}

func contagionMinSamples() int {
	if config.System.CausalContagionMinSamples > 0 {
		return config.System.CausalContagionMinSamples
	}

	return 16
}

/*
contagion measures cross-asset coupling across the subscribed universe as the median absolute
Hayashi-Yoshida correlation over symbol pairs. Crypto venues are normally correlated, so it is
the spike toward one — every asset moving as a single block during a liquidation cascade — that
flips the structural causal model into its panic regime. Returns zero when too few symbols carry
enough return history to form a stable estimate.
*/
func (causal *Causal) contagion() float64 {
	snapshots := make([]*hyReturns, 0, contagionSymbolCap)
	minSamples := contagionMinSamples()

	causal.symbols.Range(func(key, value any) bool {
		symbol := key.(string)

		if _, subscribed := causal.requested.Load(symbol); !subscribed {
			return true
		}

		state := value.(*CausalSymbol)
		snapshot := state.HYSnapshot()

		if snapshot != nil && snapshot.len() >= minSamples {
			snapshots = append(snapshots, snapshot)
		}

		return len(snapshots) < contagionSymbolCap
	})

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
