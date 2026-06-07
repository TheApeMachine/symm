package perspectives

import (
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
Preemption policy is shared by the live trader and optimizer replay — it lived
only in the trader, so replay preempted on a plain score comparison while live
demanded a regime-scaled margin, a cooldown, and one plan in flight. Replay
results scored portfolio rotation live would never perform.
*/
const (
	defaultPreemptionMargin = 1.5
	// PreemptionCooldown is the minimum spacing between preemption rounds, so
	// a burst of strong signals cannot churn the whole book in one window.
	PreemptionCooldown = 30 * time.Second

	preemptionHostileScale  = 1.5  // choppy / bearish: base 1.5 -> 2.25
	preemptionTrendingScale = 0.85 // trending / bullish: base 1.5 -> ~1.28
	preemptionMarginFloor   = 1.05 // never below a real edge over the incumbent
)

// PreemptionMargin returns the conviction multiple a challenger must beat the
// weakest incumbent by, scaled for the given regime: hostile tape demands a
// decisive edge (a swap pays a full taker round trip with no momentum to carry
// it), trending tape relaxes toward the base so capital can rotate.
func PreemptionMargin(regime types.Regime) float64 {
	margin := defaultPreemptionMargin

	if configured := viper.GetFloat64("trading.entry.preemption_margin"); configured > 1 {
		margin = configured
	}

	switch regime {
	case types.RegimeChoppy, types.RegimeBearish:
		margin *= preemptionHostileScale
	case types.RegimeTrending, types.RegimeBullish:
		margin *= preemptionTrendingScale
	}

	return max(margin, preemptionMarginFloor)
}
