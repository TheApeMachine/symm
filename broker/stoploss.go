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

/*
StoplossRegime names how an armed position is managed.
Climb uses crest-stall exit; Grind uses giveback-from-peak because tiny
up/down moves never build climb velocity the crest path needs.
*/
type StoplossRegime string

const (
	StoplossRegimeClimb StoplossRegime = "climb"
	StoplossRegimeGrind StoplossRegime = "grind"
)

type Stoploss struct {
	State         StoplossState  `json:"State"`
	Regime        StoplossRegime `json:"Regime"`
	InitialPnLPct float64        `json:"InitialPnLPct"`
	CurrentPnLPct float64        `json:"CurrentPnLPct"`
	PeakPnLPct    float64        `json:"PeakPnLPct"`
	ArmPnlPct     float64        `json:"ArmPnlPct"`
	Ascent        Ascent         `json:"Ascent"`
	ExhaustScore  float64        `json:"ExhaustScore"`
	Confirms      int            `json:"Confirms"`
}

func NewStoploss(pnlPct float64) *Stoploss {
	armPct := system.Cfg.Selection.StoplossArmPnlPct
	if armPct <= 0 {
		armPct = 1.0 // Default 1% net profit to cover round-trip taker friction
	}

	return &Stoploss{
		State:         StoplossHealthy,
		Regime:        StoplossRegimeClimb,
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
	// Peak resets on disarm so a later reclaim does not inherit stale giveback.
	if pnlPct < stoploss.ArmPnlPct {
		stoploss.State = StoplossHealthy
		stoploss.Regime = StoplossRegimeClimb
		stoploss.Ascent.Reset()
		stoploss.ExhaustScore = 0
		stoploss.Confirms = 0
		stoploss.CurrentPnLPct = pnlPct
		stoploss.PeakPnLPct = pnlPct
		return
	}

	stoploss.CurrentPnLPct = pnlPct

	if pnlPct > stoploss.PeakPnLPct {
		stoploss.PeakPnLPct = pnlPct
	}

	stoploss.State = StoplossArmed
	stoploss.Ascent.Note(pnlPct, now)
	stoploss.ExhaustScore = stoploss.Ascent.Exhaust()
	stoploss.Regime = stoploss.classify()

	sel := system.Cfg.Selection
	nearPeakFrac := sel.StoplossNearPeakFrac

	if nearPeakFrac <= 0 {
		nearPeakFrac = 0.96
	}

	rateDecayMin := sel.StoplossRateDecayMin

	if rateDecayMin <= 0 {
		rateDecayMin = 0.70
	}

	givebackPct := sel.StoplossGivebackPct

	if givebackPct <= 0 {
		givebackPct = 0.5
	}

	nearPeak := stoploss.CurrentPnLPct >= stoploss.PeakPnLPct*nearPeakFrac
	kinematicStall := stoploss.ExhaustScore >= rateDecayMin
	confirmedByMarket := marketFrictionEvidence >= sel.StoplossPumpConfirmMin
	giveback := stoploss.PeakPnLPct - stoploss.CurrentPnLPct
	offCrest := giveback >= givebackPct

	// Crest: sell while still on the high with stalled climb velocity.
	// Sample depth is not required — a fast spike never accumulates four ticks.
	crestExit := nearPeak && kinematicStall &&
		(confirmedByMarket || stoploss.ExhaustScore >= 0.90)

	// Giveback: peak was real, price has rolled off by givebackPct but is
	// still above arm — the +3% → slow bleed case crest-only never saw.
	givebackExit := offCrest

	if !(crestExit || givebackExit) {
		if stoploss.Confirms > 0 {
			stoploss.Confirms--
		}

		return
	}

	stoploss.Confirms++
	requiredConfirms := sel.StoplossConfirmMarks

	if requiredConfirms <= 0 {
		requiredConfirms = 3
	}

	if stoploss.Confirms >= requiredConfirms {
		stoploss.State = StoplossTriggered
	}
}

/*
classify marks slow, low-velocity arms as grind so exit uses giveback instead
of crest kinematics that never fire on 0.01-style chop.
*/
func (stoploss *Stoploss) classify() StoplossRegime {
	sel := system.Cfg.Selection
	minSamples := sel.StoplossGrindMinSamples
	velMax := sel.StoplossGrindVelocityMax

	if minSamples <= 0 {
		minSamples = 8
	}

	if velMax <= 0 {
		velMax = 0.05
	}

	if stoploss.Ascent.Samples >= minSamples &&
		stoploss.Ascent.PeakVelocity < velMax {
		return StoplossRegimeGrind
	}

	return StoplossRegimeClimb
}

func (stoploss *Stoploss) Triggered() bool {
	return stoploss.State == StoplossTriggered
}
