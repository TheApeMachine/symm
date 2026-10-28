package broker

import (
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/errnie"
)

// ExitState represents the lifecycle stage of an active position.
type ExitState uint8

const (
	// StateNursery: Precursor incubation. No trailing stops; give the trade breathing room.
	StateNursery ExitState = iota

	// StateIgnited: Move confirmed (> +5%). Floor locked at break-even (+1.5%).
	StateIgnited

	// StateSurfing: Parabolic expansion (> +15%). Ratchet ladder preserves the big move.
	StateSurfing

	// StateEvaporating: High alert. Bids are dissolving underneath the position.
	StateEvaporating
)

func (s ExitState) String() string {
	switch s {
	case StateNursery:
		return "NURSERY"
	case StateIgnited:
		return "IGNITED"
	case StateSurfing:
		return "SURFING"
	case StateEvaporating:
		return "EVAPORATING"
	default:
		return "UNKNOWN"
	}
}

type StopLoss struct {
	state            ExitState
	peakPnLPct       float64
	lowestPnLPct     float64
	lastPeakTime     time.Time
	evaporationTicks int
	tickCount        int
}

func NewStopLoss() *StopLoss {
	return &StopLoss{
		state: StateNursery,
	}
}

// Evaluate acts as the Single Source of Truth for position exits.
func (stoploss *StopLoss) Evaluate(position *Position) (trigger string, ok bool) {
	if position == nil || position.PnL == nil || position.PnLPct == nil || position.ClosedAt != nil {
		return "", false
	}

	stoploss.tickCount++
	now := time.Now().UTC()
	currentPct := position.PnLPct.Float64()

	// Track peak and trough percentages
	if currentPct > stoploss.peakPnLPct {
		stoploss.peakPnLPct = currentPct
		stoploss.lastPeakTime = now
	}
	if currentPct < stoploss.lowestPnLPct {
		stoploss.lowestPnLPct = currentPct
	}
	if stoploss.lastPeakTime.IsZero() {
		stoploss.lastPeakTime = position.OpenedAt
	}

	friction := frictionPts(position)
	if friction <= 0 {
		friction = 1.6 // Default ~0.8% taker fee per leg
	}

	// ------------------------------------------------------------------------
	// STATE 1: StateNursery (Precursor Incubation)
	// ------------------------------------------------------------------------
	if stoploss.state == StateNursery {
		// Hard invalidation: If the trade drops beyond the noise hurdle, cut it cleanly.
		maxLoss := math.Max(4.5, friction*2.2)
		if currentPct <= -maxLoss {
			return fmt.Sprintf("nursery invalidation: loss reached %.2f%% (max allowed -%.2f%%)", currentPct, maxLoss), true
		}

		// Stagnation timeout: If entered 30m ago and hasn't ignited (> +3%), exit gracefully.
		if now.Sub(position.OpenedAt) > 30*time.Minute && stoploss.peakPnLPct < 3.0 {
			return "nursery timeout: precursor failed to ignite within 30m", true
		}

		// Ignition transition: Once PnL clears +5.0%, the move is active.
		ignitionThreshold := math.Max(5.0, friction*2.5)
		if currentPct >= ignitionThreshold {
			stoploss.state = StateIgnited
			errnie.Info(fmt.Sprintf("[stoploss] %s: NURSERY -> IGNITED (PnL: +%.2f%%)", position.Symbol, currentPct))
		}

		return "", false
	}

	// ------------------------------------------------------------------------
	// STATE 2: StateIgnited (Spark Confirmed, Protect Principal)
	// ------------------------------------------------------------------------
	if stoploss.state == StateIgnited {
		// Break-even lock: Capital is safe (+1.5% fee coverage).
		breakEvenFloor := math.Max(1.2, friction)
		if currentPct <= breakEvenFloor {
			return fmt.Sprintf("ignited break-even stop: fell back to +%.2f%%", currentPct), true
		}

		// Surfing transition: Breakout past +15% enters the parabolic ratchet ladder.
		if stoploss.peakPnLPct >= 15.0 {
			stoploss.state = StateSurfing
			stoploss.lastPeakTime = now
			errnie.Info(fmt.Sprintf("[stoploss] %s: IGNITED -> SURFING (Peak: +%.2f%%)", position.Symbol, stoploss.peakPnLPct))
		}

		return "", false
	}

	// ------------------------------------------------------------------------
	// STATE 3 & 4: StateSurfing & StateEvaporating (The Parabolic Ratchet)
	// ------------------------------------------------------------------------
	if stoploss.state == StateSurfing || stoploss.state == StateEvaporating {
		// A. LIQUIDITY EVAPORATION / DISSOLVING BOOK CHECK
		// If bids drop below what is required to lift our lot while price pulls back:
		if position.ExitRoom != nil {
			room := position.ExitRoom.Float64()
			if room < 0.75 && currentPct < (stoploss.peakPnLPct-2.0) {
				stoploss.evaporationTicks++
				if stoploss.evaporationTicks >= 3 {
					stoploss.state = StateEvaporating
					return fmt.Sprintf("liquidity evaporation: bid exit room dissolved to %.2f (peak was +%.1f%%)", room, stoploss.peakPnLPct), true
				}
			} else {
				stoploss.evaporationTicks = 0
			}
		}

		// B. MOMENTUM STALL TIMEOUT
		// If in surfing mode and no new peak has been reached for 20m, harvest the gain.
		if now.Sub(stoploss.lastPeakTime) > 20*time.Minute && stoploss.peakPnLPct >= 20.0 {
			return fmt.Sprintf("momentum stall: 20m without new high (peak +%.1f%%, current +%.1f%%)", stoploss.peakPnLPct, currentPct), true
		}

		// C. THE RATCHET LADDER
		var floorPct float64

		switch {
		case stoploss.peakPnLPct >= 60.0:
			// Moonshot Tier (e.g. EVAA climbing towards +140%)
			// Allow 25% giveback from peak, guaranteeing at least +45%.
			giveback := stoploss.peakPnLPct * 0.25
			floorPct = math.Max(45.0, stoploss.peakPnLPct-giveback)

		case stoploss.peakPnLPct >= 30.0:
			// Strong Expansion Tier
			// Allow 30% giveback from peak, guaranteeing at least +20%.
			giveback := stoploss.peakPnLPct * 0.30
			floorPct = math.Max(20.0, stoploss.peakPnLPct-giveback)

		default:
			// Initial Surfing Tier (+15% to +30%)
			// Allow 35% giveback from peak, guaranteeing at least +8%.
			giveback := stoploss.peakPnLPct * 0.35
			floorPct = math.Max(8.0, stoploss.peakPnLPct-giveback)
		}

		if currentPct <= floorPct {
			return fmt.Sprintf("surfing ratchet stop: peak +%.1f%%, floor +%.1f%%, current +%.1f%%",
				stoploss.peakPnLPct, floorPct, currentPct), true
		}
	}

	return "", false
}

// ----------------------------------------------------------------------------
// Helpers consumed by broker/desk.go
// ----------------------------------------------------------------------------

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
		return 1.6
	}
	return position.frictionPct.Float64() * 100
}
