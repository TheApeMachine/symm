package broker

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/system"
)

type StopLoss struct {
	initialized bool
	entryPnL    float64
	peakPnL     float64
	lowestPnL   float64
}

/*
StopState is the live proximity of the two stop checks, for the dashboard.
Each ratio is current-over-threshold: ≥1 means firing. Damage is how much of
the invalidation budget the lot has already burned on its worst print.
*/
type StopState struct {
	Invalidation         float64 `json:"invalidation"`
	Trailing             float64 `json:"trailing"`
	Damage               float64 `json:"damage"`
	PeakPct              float64 `json:"peakPct"`
	CurrentPct           float64 `json:"currentPct"`
	EntryPct             float64 `json:"entryPct"`
	GivebackPct          float64 `json:"givebackPct"`
	AllowedGivebackPct   float64 `json:"allowedGivebackPct"`
	RemainingGivebackPct float64 `json:"remainingGivebackPct"`
}

func NewStopLoss() *StopLoss {
	return &StopLoss{}
}

func (stoploss *StopLoss) Update(currentPnL float64) (trigger string, ok bool) {
	if !stoploss.initialized {
		stoploss.initialized = true
		stoploss.entryPnL = currentPnL
		stoploss.lowestPnL = currentPnL
		stoploss.peakPnL = currentPnL
		return "", false
	}

	if currentPnL < stoploss.lowestPnL {
		stoploss.lowestPnL = currentPnL
	}

	if currentPnL > stoploss.peakPnL {
		stoploss.peakPnL = currentPnL
	}

	maxLoss := system.Cfg.Risk.MaxAllowedLossPct
	adverseDrop := stoploss.entryPnL - currentPnL

	if adverseDrop >= maxLoss {
		return fmt.Sprintf(
			"invalidation stop: adverse drop %.2f%% reached limit %.2f%% (current PnL: %.2f%%)",
			adverseDrop, maxLoss, currentPnL,
		), true
	}

	damageRatio := stoploss.damageRatio()

	if stoploss.peakPnL > system.Cfg.Risk.TrailPeakArmPct && currentPnL > 0 {
		giveback := stoploss.peakPnL - currentPnL
		allowed := stoploss.allowedGiveback(stoploss.peakPnL, damageRatio)

		if giveback >= allowed {
			return fmt.Sprintf(
				"trailing take-profit: peak +%.2f%% gave back %.2f%% (allowed: %.2f%%, damage: %.0f%%)",
				stoploss.peakPnL, giveback, allowed, damageRatio*100,
			), true
		}
	}

	return "", false
}

/*
State snapshots stop proximity after Update has advanced extremes for this mark.
*/
func (stoploss *StopLoss) State(currentPnL float64) StopState {
	state := StopState{CurrentPct: currentPnL}

	if stoploss == nil || !stoploss.initialized {
		return state
	}

	state.EntryPct = stoploss.entryPnL
	state.PeakPct = stoploss.peakPnL
	state.Damage = stoploss.damageRatio()

	adverse := stoploss.entryPnL - currentPnL
	if adverse > 0 {
		state.Invalidation = adverse / system.Cfg.Risk.MaxAllowedLossPct
	}

	if stoploss.peakPnL > system.Cfg.Risk.TrailPeakArmPct && currentPnL > 0 {
		giveback := math.Max(0, stoploss.peakPnL-currentPnL)
		allowed := stoploss.allowedGiveback(stoploss.peakPnL, state.Damage)
		state.GivebackPct = giveback
		state.AllowedGivebackPct = allowed
		state.RemainingGivebackPct = math.Max(0, allowed-giveback)

		if allowed > 0 {
			state.Trailing = giveback / allowed
		}
	}

	return state
}

func (stoploss *StopLoss) damageRatio() float64 {
	if stoploss == nil {
		return 0
	}

	drawdown := stoploss.entryPnL - stoploss.lowestPnL
	return math.Min(1.0, math.Max(0.0, drawdown/system.Cfg.Risk.MaxAllowedLossPct))
}

func (stoploss *StopLoss) allowedGiveback(peak, damage float64) float64 {
	allowed := system.Cfg.Risk.TrailBaseGivebackPct * (2.0 / (peak + 1.0))
	allowed = allowed * (1.0 - (0.5 * damage))

	if allowed < system.Cfg.Risk.TrailMinGivebackPct {
		return system.Cfg.Risk.TrailMinGivebackPct
	}

	return allowed
}
