package stoploss

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/system"
)

type StopLoss struct {
	initialized  bool
	entryPnL     float64
	peakPnL      float64
	lowestPnL    float64
	budget       *Budget
	baseGiveback float64
	minGiveback  float64
}

func NewStopLoss() *StopLoss {
	risk := system.Cfg.Risk

	return &StopLoss{
		budget:       NewBudget().Arm(0),
		baseGiveback: risk.TrailBaseGivebackPct,
		minGiveback:  risk.TrailMinGivebackPct,
	}
}

func (stoploss *StopLoss) Update(currentPnL float64) (trigger string, ok bool) {
	if !stoploss.initialized {
		// Same anchor as Structural: PnL% is already vs entry fill.
		stoploss.initialized = true
		stoploss.entryPnL = 0
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

	maxLoss := stoploss.budget.maxLoss
	adverseDrop := stoploss.entryPnL - currentPnL

	if adverseDrop >= maxLoss {
		return fmt.Sprintf(
			"invalidation stop: adverse drop %.2f%% reached limit %.2f%% (current PnL: %.2f%%)",
			adverseDrop, maxLoss, currentPnL,
		), true
	}

	damageRatio := stoploss.damageRatio()

	if stoploss.peakPnL > stoploss.budget.armPct && currentPnL > 0 {
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
func (stoploss *StopLoss) State(currentPnL float64) State {
	state := State{CurrentPct: currentPnL}

	if !stoploss.initialized {
		return state
	}

	state.EntryPct = stoploss.entryPnL
	state.PeakPct = stoploss.peakPnL
	state.Damage = stoploss.damageRatio()
	state.MaxLossPct = stoploss.budget.maxLoss
	state.FloorPct = stoploss.ActiveFloor(currentPnL)
	state.RoomPct = currentPnL - state.FloorPct
	state.Proximity = stoploss.Proximity(currentPnL)

	adverse := stoploss.entryPnL - currentPnL
	if adverse > 0 && stoploss.budget.maxLoss > 0 {
		state.Invalidation = adverse / stoploss.budget.maxLoss
	}

	if stoploss.peakPnL > stoploss.budget.armPct && currentPnL > 0 {
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

/*
ActiveFloor is the PnL% rail that can actually settle the lot at this mark.
Trailing only counts while still green — same gate as Update — so the chart
band matches what will fire. Red lots fall back to the invalidation cut.
*/
func (stoploss *StopLoss) ActiveFloor(currentPnL float64) float64 {
	if !stoploss.initialized {
		return 0
	}

	floor := stoploss.entryPnL - stoploss.budget.maxLoss

	if stoploss.peakPnL > stoploss.budget.armPct && currentPnL > 0 {
		trail := stoploss.peakPnL - stoploss.allowedGiveback(stoploss.peakPnL, stoploss.damageRatio())
		if trail > floor {
			floor = trail
		}
	}

	return floor
}

/*
Proximity is how close the mark is to the active stop: 0 idle, 1 firing.
*/
func (stoploss *StopLoss) Proximity(currentPnL float64) float64 {
	if !stoploss.initialized {
		return 0
	}

	maxLoss := stoploss.budget.maxLoss
	proximity := 0.0
	adverse := stoploss.entryPnL - currentPnL

	if adverse > 0 && maxLoss > 0 {
		proximity = adverse / maxLoss
	}

	if stoploss.peakPnL > stoploss.budget.armPct && currentPnL > 0 {
		allowed := stoploss.allowedGiveback(stoploss.peakPnL, stoploss.damageRatio())
		if allowed > 0 {
			giveback := math.Max(0, stoploss.peakPnL-currentPnL)
			trail := giveback / allowed
			if trail > proximity {
				proximity = trail
			}
		}
	}

	return proximity
}

func (stoploss *StopLoss) damageRatio() float64 {
	maxLoss := stoploss.budget.maxLoss
	if maxLoss <= 0 {
		return 0
	}

	drawdown := stoploss.entryPnL - stoploss.lowestPnL
	return math.Min(1.0, math.Max(0.0, drawdown/maxLoss))
}

func (stoploss *StopLoss) allowedGiveback(peak, damage float64) float64 {
	allowed := stoploss.baseGiveback * (2.0 / (peak + 1.0))
	allowed = allowed * (1.0 - (0.5 * damage))

	if allowed < stoploss.minGiveback {
		return stoploss.minGiveback
	}

	return allowed
}

/*
TrailMachine is the learning stop's locked extremes for resume.
*/
type TrailMachine struct {
	Initialized  bool    `json:"initialized"`
	EntryPnL     float64 `json:"entry_pnl"`
	PeakPnL      float64 `json:"peak_pnl"`
	LowestPnL    float64 `json:"lowest_pnl"`
	MaxLoss      float64 `json:"max_loss"`
	ArmPct       float64 `json:"arm_pct"`
	BaseGiveback float64 `json:"base_giveback"`
	MinGiveback  float64 `json:"min_giveback"`
}

func (stoploss *StopLoss) Machine() TrailMachine {
	maxLoss, armPct := 0.0, 0.0

	if stoploss.budget != nil {
		maxLoss = stoploss.budget.maxLoss
		armPct = stoploss.budget.armPct
	}

	return TrailMachine{
		Initialized:  stoploss.initialized,
		EntryPnL:     stoploss.entryPnL,
		PeakPnL:      stoploss.peakPnL,
		LowestPnL:    stoploss.lowestPnL,
		MaxLoss:      maxLoss,
		ArmPct:       armPct,
		BaseGiveback: stoploss.baseGiveback,
		MinGiveback:  stoploss.minGiveback,
	}
}

func (stoploss *StopLoss) Resume(machine TrailMachine) {
	if !machine.Initialized {
		return
	}

	if stoploss.budget == nil {
		stoploss.budget = NewBudget().Arm(0)
	}

	stoploss.initialized = true
	stoploss.entryPnL = machine.EntryPnL
	stoploss.peakPnL = machine.PeakPnL
	stoploss.lowestPnL = machine.LowestPnL
	stoploss.baseGiveback = machine.BaseGiveback
	stoploss.minGiveback = machine.MinGiveback
	stoploss.budget.maxLoss = machine.MaxLoss
	stoploss.budget.armPct = machine.ArmPct
	stoploss.budget.armed = machine.MaxLoss > 0
}
