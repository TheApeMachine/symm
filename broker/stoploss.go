package broker

import (
	"math"
	"time"

	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/symm/system"
)

type StoplossState uint8

const (
	StoplossStateInit StoplossState = iota
	StoplossHealthy                 // Closer to profit than to the floor.
	StoplossDangerous               // Closer to the floor than to profit.
	StoplossCritical                // Closer to the floor than entry drag allows.
	StoplossProtected               // Cleared the mirror profit target.
	StoplossTriggered               // Hit the active floor; exit.
)

/*
Stoploss:

	Hard rails (absolute, centered on 0):
	  MaxLossPct      = -MaxAllowedLossPct
	  ProfitTargetPct = +MaxAllowedLossPct

	Entry PnL is already fee/spread-negative. Green is not protected until
	ProfitTargetPct.

	Active FloorPnLPct starts at MaxLossPct and breathes with the mark:
	  up-move   → widen  by |Δ| × pace × (1 - nearFloor)
	  down-move → tighten by |Δ| × pace × max(nearFloor, nearProfit)
	pace is mark-arrival speed vs a TimeElastic baseline (faster clusters pull harder).
	Floor never breaches MaxLossPct. Once Protected, floor never sits below 0.
*/
type Stoploss struct {
	State           StoplossState
	InitialPnLPct   float64
	MaxLossPct      float64
	ProfitTargetPct float64
	PreviousPnLPct  float64
	CurrentPnLPct   float64
	PeakPnLPct      float64
	FloorPnLPct     float64
	PreviousTime    time.Time
	CurrentTime     time.Time
	pace            *adaptive.TimeElastic
}

func NewStoploss(PnLPct float64) *Stoploss {
	maxLoss := system.Cfg.Risk.MaxAllowedLossPct
	now := time.Now()

	return &Stoploss{
		State:           StoplossStateInit,
		InitialPnLPct:   PnLPct,
		MaxLossPct:      -maxLoss,
		ProfitTargetPct: maxLoss,
		PreviousPnLPct:  PnLPct,
		CurrentPnLPct:   PnLPct,
		PeakPnLPct:      PnLPct,
		FloorPnLPct:     -maxLoss,
		PreviousTime:    now,
		CurrentTime:     now,
		pace: adaptive.NewTimeElastic(adaptive.TimeElasticConfig{
			Halflife: system.Cfg.Risk.StopPaceHalflife,
			Epsilon:  system.Cfg.Risk.StopPaceEpsilon,
		}),
	}
}

func (stoploss *Stoploss) Update(PnLPct float64) {
	if stoploss.State == StoplossTriggered {
		return
	}

	stoploss.PreviousPnLPct = stoploss.CurrentPnLPct
	stoploss.CurrentPnLPct = PnLPct
	stoploss.PreviousTime = stoploss.CurrentTime
	stoploss.CurrentTime = time.Now()

	if PnLPct > stoploss.PeakPnLPct {
		stoploss.PeakPnLPct = PnLPct
	}

	stoploss.breathe(PnLPct)

	if stoploss.FloorPnLPct < stoploss.MaxLossPct {
		stoploss.FloorPnLPct = stoploss.MaxLossPct
	}

	if PnLPct <= stoploss.FloorPnLPct {
		stoploss.State = StoplossTriggered
		return
	}

	if PnLPct >= stoploss.ProfitTargetPct {
		stoploss.State = StoplossProtected

		if stoploss.FloorPnLPct < 0 {
			stoploss.FloorPnLPct = 0
		}

		return
	}

	stoploss.State = stoploss.health(PnLPct)
}

func (stoploss *Stoploss) Triggered() bool {
	return stoploss.State == StoplossTriggered
}

func (stoploss *Stoploss) breathe(pnl float64) {
	delta := pnl - stoploss.PreviousPnLPct
	
	if delta == 0 {
		return
	}

	span := stoploss.ProfitTargetPct - stoploss.MaxLossPct
	
	if span <= 0 {
		return
	}

	nearFloor := 1 - math.Max(0, math.Min(1, (pnl-stoploss.FloorPnLPct)/span))
	nearProfit := 1 - math.Max(0, math.Min(1, (stoploss.ProfitTargetPct-pnl)/span))

	pace := stoploss.markPace()
	move := math.Abs(delta) * pace

	if delta > 0 {
		stoploss.FloorPnLPct -= move * (1 - nearFloor)
		return
	}

	stoploss.FloorPnLPct += move * math.Max(nearFloor, nearProfit)
}

/*
markPace is instant mark speed relative to the TimeElastic tempo baseline.
*/
func (stoploss *Stoploss) markPace() float64 {
	interval := stoploss.CurrentTime.Sub(stoploss.PreviousTime)

	if interval <= 0 {
		return 1
	}

	instantSpeed := float64(time.Second) / float64(interval)
	
	out, err := stoploss.pace.Measure(adaptive.TimedValue{
		Value: instantSpeed,
		At:    stoploss.CurrentTime,
	})

	if err != nil || !out.Ready {
		return 1
	}

	return out.Value
}

func (stoploss *Stoploss) health(pnl float64) StoplossState {
	roomToFloor := pnl - stoploss.FloorPnLPct
	roomToProfit := stoploss.ProfitTargetPct - pnl

	if roomToFloor < roomToProfit {
		entryDrag := math.Abs(stoploss.InitialPnLPct)

		if entryDrag > 0 && roomToFloor < entryDrag {
			return StoplossCritical
		}

		return StoplossDangerous
	}

	return StoplossHealthy
}
