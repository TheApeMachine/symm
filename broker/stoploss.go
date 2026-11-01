package broker

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
	budget       *LossBudget
	baseGiveback float64
	minGiveback  float64
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
	// FloorPct is the exit rail that can fire at CurrentPct (trail only while green).
	FloorPct float64 `json:"floorPct"`
	// Proximity is 0..1+ toward the active stop (≥1 means at/through the rail).
	Proximity float64 `json:"proximity"`
	// RoomPct is how many PnL points remain above the active floor.
	RoomPct float64 `json:"roomPct"`
	// MaxLossPct is the frozen invalidation width for this lot.
	MaxLossPct float64 `json:"maxLossPct"`
}

func NewStopLoss() *StopLoss {
	return &StopLoss{budget: NewLossBudget()}
}

/*
BindBudget shares the lot's frozen LossBudget. Nil is ignored.
*/
func (stoploss *StopLoss) BindBudget(budget *LossBudget) {
	if stoploss == nil || budget == nil {
		return
	}

	stoploss.budget = budget
}

/*
BindGiveback overlays trail giveback statics (used by LearningStop rails).
Zero keeps the risk config defaults.
*/
func (stoploss *StopLoss) BindGiveback(base, min float64) {
	if stoploss == nil {
		return
	}

	stoploss.baseGiveback = base
	stoploss.minGiveback = min
}

func (stoploss *StopLoss) ensureBudget() *LossBudget {
	if stoploss == nil {
		return NewLossBudget().Arm(0)
	}

	if stoploss.budget == nil {
		stoploss.budget = NewLossBudget()
	}

	if !stoploss.budget.Armed() {
		stoploss.budget.Arm(0)
	}

	return stoploss.budget
}

func (stoploss *StopLoss) Update(currentPnL float64) (trigger string, ok bool) {
	budget := stoploss.ensureBudget()

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

	maxLoss := budget.MaxLoss()
	adverseDrop := stoploss.entryPnL - currentPnL

	if adverseDrop >= maxLoss {
		return fmt.Sprintf(
			"invalidation stop: adverse drop %.2f%% reached limit %.2f%% (current PnL: %.2f%%)",
			adverseDrop, maxLoss, currentPnL,
		), true
	}

	damageRatio := stoploss.damageRatio()

	if stoploss.peakPnL > budget.ArmPct() && currentPnL > 0 {
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

	budget := stoploss.ensureBudget()

	state.EntryPct = stoploss.entryPnL
	state.PeakPct = stoploss.peakPnL
	state.Damage = stoploss.damageRatio()
	state.MaxLossPct = budget.MaxLoss()
	state.FloorPct = stoploss.ActiveFloor(currentPnL)
	state.RoomPct = currentPnL - state.FloorPct
	state.Proximity = stoploss.Proximity(currentPnL)

	adverse := stoploss.entryPnL - currentPnL
	if adverse > 0 && budget.MaxLoss() > 0 {
		state.Invalidation = adverse / budget.MaxLoss()
	}

	if stoploss.peakPnL > budget.ArmPct() && currentPnL > 0 {
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
	if stoploss == nil || !stoploss.initialized {
		return 0
	}

	budget := stoploss.ensureBudget()
	floor := stoploss.entryPnL - budget.MaxLoss()

	if stoploss.peakPnL > budget.ArmPct() && currentPnL > 0 {
		trail := stoploss.peakPnL - stoploss.allowedGiveback(stoploss.peakPnL, stoploss.damageRatio())
		if trail > floor {
			floor = trail
		}
	}

	return floor
}

/*
Proximity is how close the mark is to the active stop: 0 idle, 1 firing.
Uses the same checks Update uses (invalidation always; trail only while green).
*/
func (stoploss *StopLoss) Proximity(currentPnL float64) float64 {
	if stoploss == nil || !stoploss.initialized {
		return 0
	}

	budget := stoploss.ensureBudget()
	maxLoss := budget.MaxLoss()

	proximity := 0.0
	adverse := stoploss.entryPnL - currentPnL
	if adverse > 0 && maxLoss > 0 {
		proximity = adverse / maxLoss
	}

	if stoploss.peakPnL > budget.ArmPct() && currentPnL > 0 {
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
	if stoploss == nil {
		return 0
	}

	budget := stoploss.ensureBudget()
	maxLoss := budget.MaxLoss()
	if maxLoss <= 0 {
		return 0
	}

	drawdown := stoploss.entryPnL - stoploss.lowestPnL
	return math.Min(1.0, math.Max(0.0, drawdown/maxLoss))
}

func (stoploss *StopLoss) allowedGiveback(peak, damage float64) float64 {
	base := system.Cfg.Risk.TrailBaseGivebackPct
	min := system.Cfg.Risk.TrailMinGivebackPct

	if stoploss.baseGiveback > 0 {
		base = stoploss.baseGiveback
	}

	if stoploss.minGiveback > 0 {
		min = stoploss.minGiveback
	}

	allowed := base * (2.0 / (peak + 1.0))
	allowed = allowed * (1.0 - (0.5 * damage))

	if allowed < min {
		return min
	}

	return allowed
}
