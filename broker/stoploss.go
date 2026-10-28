package broker

import (
	"fmt"
	"math"
	"time"
)

type StopLoss struct {
	initialized      bool
	entryPnLPct      float64
	peakPnLPct       float64
	lowestPnLPct     float64
	tickCount        int
	evaporationTicks int
}

func NewStopLoss() *StopLoss {
	return &StopLoss{}
}

/*
Evaluate is the single source of truth for position exits.
It executes four checks:
 1. Physical Book Liquidity: ExitRoom < 1.0 means our order cannot be absorbed at the touch.
 2. Invalidation Stop: Cut loss when PnL crosses the Causal MCTS learned cut threshold.
 3. Trailing Profit Stop: Exit when giveback from peak crosses the Causal MCTS learned trailing threshold.
 4. Empirical Move Expiry: Exit if position duration exceeds the empirical winning excursion horizon without clearing costs.
*/
func (stoploss *StopLoss) Evaluate(position *Position) (trigger string, ok bool) {
	if position == nil || position.PnL == nil || position.PnLPct == nil || position.ClosedAt != nil {
		return "", false
	}

	currentPct := position.PnLPct.Float64()

	if !stoploss.initialized {
		stoploss.initialized = true
		stoploss.entryPnLPct = currentPct
		stoploss.peakPnLPct = currentPct
		stoploss.lowestPnLPct = currentPct
		return "", false
	}

	stoploss.tickCount++

	if currentPct > stoploss.peakPnLPct {
		stoploss.peakPnLPct = currentPct
	}

	if currentPct < stoploss.lowestPnLPct {
		stoploss.lowestPnLPct = currentPct
	}

	friction := frictionPts(position)

	learnedCut := position.NurseryCutPct
	learnedTrail := position.ExitPullbackPct
	maxLifetimeNs := position.MaxLifetimeNs

	// ------------------------------------------------------------------------
	// 1. CAUSAL MCTS LEARNED INVALIDATION STOP
	// ------------------------------------------------------------------------
	cutTolerance := friction

	if learnedCut != nil && learnedCut.Sign() > 0 {
		cutTolerance = math.Max(friction, learnedCut.Float64())
	}

	adverseMove := stoploss.entryPnLPct - currentPct

	if adverseMove >= cutTolerance {
		return fmt.Sprintf("invalidation stop: adverse drop %.2f%% exceeded causal cut threshold %.2f%% (current PnL %.2f%%)",
			adverseMove, cutTolerance, currentPct), true
	}

	// Calculate how close the trade came to being wiped out before bouncing
	maxAdverseMove := stoploss.entryPnLPct - stoploss.lowestPnLPct
	damageRatio := 0.0

	if cutTolerance > 0 {
		damageRatio = maxAdverseMove / cutTolerance
	}

	isDamaged := damageRatio >= 0.75 // Consumed 75% or more of its risk budget

	// ------------------------------------------------------------------------
	// 2. REAL PHYSICAL LIQUIDITY EVAPORATION
	// Evaluated via full-lot Executable VWAP slippage, not fleeting micro-orders:
	// A. Book depth is physically incapable of filling our lot (ExitVWAP == nil).
	// B. Slippage to exit our full lot exceeds the venue friction hurdle.
	// C. Sustained across multiple ticks while in profit.
	// ------------------------------------------------------------------------
	if stoploss.peakPnLPct > 0 && position.Mark != nil && position.Mark.Sign() > 0 {
		evaporated := false
		var reason string

		if position.ExitVWAP == nil || position.ExitVWAP.Sign() <= 0 {
			// The bids in the book literally cannot fill our lot size
			evaporated = true
			reason = "order book depth cannot fill lot size"
		} else {
			markVal := position.Mark.Float64()
			vwapVal := position.ExitVWAP.Float64()

			// Full-lot liquidation slippage in percent
			slippagePct := ((markVal - vwapVal) / markVal) * 100.0

			// If slippage to exit exceeds round-trip friction, exiting walks into a hollow book
			if slippagePct > friction && currentPct < stoploss.peakPnLPct {
				evaporated = true
				reason = fmt.Sprintf("full-lot slippage %.2f%% exceeds friction %.2f%%", slippagePct, friction)
			}
		}

		if evaporated {
			stoploss.evaporationTicks++
			// Require persistence: genuine book pulls remain across ticks; bot flickers clear in 1 tick
			if stoploss.evaporationTicks >= 3 {
				return fmt.Sprintf(
					"liquidity evaporation: %s (peak +%.2f%%, current PnL %.2f%%)",
					reason, stoploss.peakPnLPct, currentPct,
				), true
			}
		} else {
			stoploss.evaporationTicks = 0
		}
	}

	// ------------------------------------------------------------------------
	// 3. CAUSAL MCTS LEARNED TRAILING PROFIT STOP
	// ------------------------------------------------------------------------
	if stoploss.peakPnLPct > 0 && learnedTrail != nil && learnedTrail.Sign() > 0 {
		giveback := stoploss.peakPnLPct - currentPct
		baseTrail := math.Max(friction, learnedTrail.Float64())

		// If the trade almost died but miraculously went green, the premise is broken.
		// We exit on the very first sign of momentum faltering.
		if isDamaged {
			survivalMargin := math.Max(0.0, 1.0-damageRatio)
			baseTrail = friction + ((baseTrail - friction) * survivalMargin)
		}

		// --- PARABOLIC PROFIT RATCHET ---
		// Compresses the allowed giveback from the base trail down towards the
		// friction floor using an inverse-square curve based on peak multiples.
		compression := math.Max(1.0, stoploss.peakPnLPct/baseTrail)
		dynamicTrail := friction + ((baseTrail - friction) / math.Sqrt(compression))

		if giveback >= dynamicTrail {
			reason := "ratcheted trailing stop"
			
			if isDamaged {
				reason = "damaged recovery ratcheted stop"
			}

			return fmt.Sprintf("%s: peak +%.2f%%, giveback %.2f%% reached dynamic trail %.2f%% (base %.2f%%, net PnL %.2f%%)",
				reason, stoploss.peakPnLPct, giveback, dynamicTrail, baseTrail, currentPct,
			), true
		}
	}

	// ------------------------------------------------------------------------
	// 4. EMPIRICAL MOVE EXPIRY
	// ------------------------------------------------------------------------
	if maxLifetimeNs > 0 && !position.OpenedAt.IsZero() {
		elapsed := time.Since(position.OpenedAt).Nanoseconds()
		if elapsed > maxLifetimeNs && stoploss.peakPnLPct <= 0 {
			return fmt.Sprintf(
				"move expired: elapsed %s exceeded empirical winning horizon without reaching profit",
				time.Duration(elapsed).Round(time.Second),
			), true
		}
	}

	return "", false
}

func profitLockArmed(position *Position) bool {
	if position == nil || position.PeakPnLPct == nil {
		return false
	}
	friction := frictionPts(position)
	return position.PeakPnLPct.Float64() > friction
}

func watchGreenBook(position *Position) bool {
	if position == nil || position.PnL == nil {
		return false
	}
	return position.PnL.Sign() > 0
}

func frictionPts(position *Position) float64 {
	if position == nil || position.frictionPct == nil {
		return 0
	}
	return position.frictionPct.Float64() * 100
}
