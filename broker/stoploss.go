package broker

import (
	"time"

	"github.com/theapemachine/symm/system"
)

type StoplossState int

const (
	StoplossHealthy StoplossState = iota
	StoplossArmed
	stoplossLegacyTakeProfit
	StoplossTriggered
)

type Stoploss struct {
	State         StoplossState `json:"State"`
	InitialPnLPct float64       `json:"InitialPnLPct"`
	CurrentPnLPct float64       `json:"CurrentPnLPct"`
	PeakPnLPct    float64       `json:"PeakPnLPct"`
	ArmPnlPct     float64       `json:"ArmPnlPct"`
	Ascent        Ascent        `json:"Ascent"`
	ExhaustScore  float64       `json:"ExhaustScore"`
	Confirms      int           `json:"Confirms"`
}

func NewStoploss(pnlPct float64) *Stoploss {
	armPct := system.Cfg.Selection.StoplossArmPnlPct
	if armPct <= 0 {
		armPct = 1.0 // Default 1% net profit to cover round-trip taker friction
	}

	return &Stoploss{
		State:         StoplossHealthy,
		InitialPnLPct: pnlPct,
		CurrentPnLPct: pnlPct,
		PeakPnLPct:    pnlPct,
		ArmPnlPct:     armPct,
	}
}

/*
Update evaluates market progress.
- pnlPct: fee-net PnL percentage.
- marketFrictionEvidence: optional external signal (e.g. CVD absorption, Hawkes decay, or pumpdump).
*/
func (stoploss *Stoploss) Update(pnlPct float64, marketFrictionEvidence float64) {
	if stoploss.State == StoplossTriggered {
		return
	}

	now := time.Now()

	// RULE 1: Never exit at a loss or before arming. Hold until profit.
	if pnlPct < stoploss.ArmPnlPct {
		stoploss.State = StoplossHealthy
		stoploss.Ascent.Reset()
		stoploss.ExhaustScore = 0
		stoploss.Confirms = 0
		stoploss.CurrentPnLPct = pnlPct
		return
	}

	stoploss.CurrentPnLPct = pnlPct

	if pnlPct > stoploss.PeakPnLPct {
		stoploss.PeakPnLPct = pnlPct
	}

	// Arm once we are safely in fee-net profit
	stoploss.State = StoplossArmed

	// Feed smoothed kinematic ascent tracker
	stoploss.Ascent.Note(pnlPct, now)
	stoploss.ExhaustScore = stoploss.Ascent.Exhaust()

	sel := system.Cfg.Selection
	nearPeakFrac := sel.StoplossNearPeakFrac

	if nearPeakFrac <= 0 {
		nearPeakFrac = 0.96 // Must be within 4% of peak (on the crest)
	}

	rateDecayMin := sel.StoplossRateDecayMin

	if rateDecayMin <= 0 {
		rateDecayMin = 0.70 // Climb velocity slowed by at least 70%
	}

	// RULE 2: NOT give-back based.
	// We MUST be near the peak (on the ascent or plateau, not tumbling down).
	nearPeak := stoploss.CurrentPnLPct >= (stoploss.PeakPnLPct * nearPeakFrac)

	// RULE 3: Kinematic deceleration confirmation
	kinematicStall := stoploss.ExhaustScore >= rateDecayMin

	// Corroboration: either strong kinematic stall with sufficient sample history,
	// or kinematic stall confirmed by external microstructure/order flow friction.
	hasSampleDepth := stoploss.Ascent.Samples >= 4
	confirmedByMarket := marketFrictionEvidence >= sel.StoplossPumpConfirmMin

	if nearPeak && kinematicStall && hasSampleDepth && (confirmedByMarket || stoploss.ExhaustScore >= 0.90) {
		stoploss.Confirms++
	} else {
		// Decay confirms gracefully rather than zeroing out on a single outlier tick
		if stoploss.Confirms > 0 {
			stoploss.Confirms--
		}

		return
	}

	requiredConfirms := sel.StoplossConfirmMarks

	if requiredConfirms <= 0 {
		requiredConfirms = 3
	}

	if stoploss.Confirms >= requiredConfirms {
		stoploss.State = StoplossTriggered
	}
}

func (stoploss *Stoploss) Triggered() bool {
	return stoploss.State == StoplossTriggered
}