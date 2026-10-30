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

/*
StopState is the live proximity of every stop check on a lot, for the
dashboard. Each ratio is current-value-over-threshold: >1.0 means the check
is firing, 0.8-1.0 means it is about to, <0.8 means armed but far. The UI
paints the four meters and the operator can see *why* a lot is still held
or is about to exit, per tick.
*/
type StopState struct {
	// Invalidation: adverse move (entry - current) vs the learned cut.
	Invalidation float64 `json:"invalidation"`
	// Trailing: giveback from peak vs the learned trail.
	Trailing float64 `json:"trailing"`
	// Evaporation: full-lot slippage vs 2x friction (3-tick persistent).
	Evaporation float64 `json:"evaporation"`
	// Expiry: bars held vs the empirical winning horizon.
	Expiry float64 `json:"expiry"`
	// Peak and current in percent, so the UI can show the trajectory.
	PeakPct    float64 `json:"peakPct"`
	CurrentPct float64 `json:"currentPct"`
	BarsHeld   int     `json:"barsHeld"`
	MaxBars    int     `json:"maxBars"`
}

func (stoploss *StopLoss) State(position *Position) StopState {
	state := StopState{BarsHeld: int(position.BarsHeld), MaxBars: position.MaxLifetimeBars}

	if position == nil || position.PnLPct == nil {
		return state
	}

	current := position.PnLPct.Float64()
	state.CurrentPct = current
	if position.PeakPnLPct != nil {
		state.PeakPct = position.PeakPnLPct.Float64()
	}

	friction := frictionPts(position)

	// 1. Invalidation: how much of the cut budget the adverse move has consumed.
	adverse := stoploss.entryPnLPct - current
	if learnedCut := position.NurseryCutPct; learnedCut != nil && learnedCut.Sign() > 0 {
		tol := math.Max(friction, learnedCut.Float64())
		if tol > 0 {
			state.Invalidation = adverse / tol
		}
	} else if friction > 0 && adverse > 0 {
		state.Invalidation = adverse / friction
	}

	// 2. Trailing: giveback from peak vs the learned trail (only armed once green).
	if state.PeakPct > 0 {
		giveback := state.PeakPct - current
		if learnedTrail := position.ExitPullbackPct; learnedTrail != nil && learnedTrail.Sign() > 0 {
			base := math.Max(friction, learnedTrail.Float64())
			if base > 0 {
				state.Trailing = giveback / base
			}
		} else if friction > 0 && giveback > 0 {
			state.Trailing = giveback / friction
		}
	}

	// 3. Evaporation: full-lot slippage vs 2x friction.
	if position.Mark != nil && position.Mark.Sign() > 0 && position.ExitVWAP != nil && position.ExitVWAP.Sign() > 0 {
		mark := position.Mark.Float64()
		vwap := position.ExitVWAP.Float64()
		if mark > 0 {
			slipPct := ((mark - vwap) / mark) * 100.0
			if friction > 0 {
				state.Evaporation = slipPct / (2.0 * friction)
			}
		}
	}

	// 4. Expiry: bars held vs the empirical winning horizon.
	if position.MaxLifetimeBars > 0 && state.PeakPct <= 0 {
		state.Expiry = float64(state.BarsHeld) / float64(position.MaxLifetimeBars)
	}

	return state
}

func NewStopLoss() *StopLoss {
	return &StopLoss{}
}

/*
Evaluate is the single source of truth for position exits. It runs four checks
in order, all in the same tape/mark timebase (no wall-clock rules):
 1. Invalidation Stop: cut loss when PnL crosses the Causal MCTS learned cut.
 2. Liquidity Evaporation: the full-lot exit walk cannot fill, or slippage
    doubles the round-trip friction, sustained across 3 marks — regardless of
    PnL sign, so an underwater lot is protected the same as one in profit.
 3. Trailing Profit Stop: exit when giveback from peak crosses the learned trail.
 4. Move Expiry: exit when the lot has held past the empirical winning horizon
    (in bars) without ever reaching profit.
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
	// B. Slippage is extreme: more than double the friction hurdle.
	// C. Sustained across multiple ticks, in or out of profit.
	//
	// It runs regardless of PnL sign: a lot underwater (peak <= 0) is exactly the
	// one the 3-tick persistence must protect, because the touch can pull to a
	// sub-1 ExitRoom on a single mark and fully recover in the next (a one-order
	// step-back, not a dissolved book). The 2x-friction slippage hurdle and the
	// >= 3 tick persistence are what keep a transient flicker from exiting;
	// only a book that stays pulled for 3 consecutive marks fires.
	// ------------------------------------------------------------------------
	if position.Mark != nil && position.Mark.Sign() > 0 {
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

			// True evaporation: slippage more than doubles the round-trip friction.
			// A slippage that merely exceeds friction (e.g. 1.89% vs 1.88%) is
			// the normal state of a thin alt book, not a book pull.
			evaporationHurdle := friction * 2.0

			if slippagePct > evaporationHurdle && currentPct < stoploss.peakPnLPct {
				evaporated = true
				reason = fmt.Sprintf("full-lot slippage %.2f%% exceeds 2x friction %.2f%%", slippagePct, evaporationHurdle)
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
		// Tightens giveback only after the move has clearly cleared the base
		// trail; small winners keep the full learned trail so profit is not
		// clipped at the first green flicker.
		dynamicTrail := baseTrail
		if stoploss.peakPnLPct >= 2*baseTrail {
			compression := stoploss.peakPnLPct / baseTrail
			dynamicTrail = friction + ((baseTrail - friction) / math.Sqrt(compression))
		}

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
	// 4. EMPIRICAL MOVE EXPIRY (bar-count, not wall-clock)
	// ------------------------------------------------------------------------
	// maxLifetimeBars is the 75th-percentile bar index of winning excursions
	// derived from the trace peak in observePolicy. A bar is one reprice event,
	// so a slow microcap's 8-bar move is not expired a third faster than a hot
	// one's. Falls back to the legacy wall-clock horizon only for a position
	// that has not yet repriced once.
	barsHeld := int(position.BarsHeld)
	if position.MaxLifetimeBars > 0 && barsHeld > position.MaxLifetimeBars && stoploss.peakPnLPct <= 0 {
		return fmt.Sprintf(
			"move expired: %d bars exceeded empirical winning horizon %d without reaching profit",
			barsHeld, position.MaxLifetimeBars,
		), true
	}

	if maxLifetimeNs > 0 && !position.OpenedAt.IsZero() && position.BarsHeld == 0 {
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

func (stoploss *StopLoss) ProfitLocked(position *Position) bool {
	friction := frictionPts(position)
	return position.PeakPnLPct.Float64() > friction
}

// func watchGreenBook(position *Position) bool {
// 	if position == nil || position.PnL == nil {
// 		return false
// 	}
// 	return position.PnL.Sign() > 0
// }

func frictionPts(position *Position) float64 {
	if position == nil || position.frictionPct == nil {
		return 0
	}
	return position.frictionPct.Float64() * 100
}
