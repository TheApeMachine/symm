package broker

import (
	"math"
	"time"
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
Climb uses crest + exhaust-scaled trail; Grind exits only with market friction.
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

/*
NewStoploss arms against armPct — live book friction / fee floor in PnL points.
*/
func NewStoploss(pnlPct, armPct float64) *Stoploss {
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
Update evaluates progress from this position's own climb and ascent.
Exit when ExhaustScore + marketFrictionEvidence >= 1 (friction covers remaining
climb velocity), never on unchanged PnL or before Samples >= 2.
*/
func (stoploss *Stoploss) Update(pnlPct float64, marketFrictionEvidence float64) {
	if stoploss.State == StoplossTriggered {
		return
	}

	now := time.Now()

	if stoploss.ArmPnlPct <= 0 || pnlPct < stoploss.ArmPnlPct {
		stoploss.State = StoplossHealthy
		stoploss.Regime = StoplossRegimeClimb
		stoploss.Ascent.Seed(pnlPct, now)
		stoploss.ExhaustScore = 0
		stoploss.Confirms = 0
		stoploss.CurrentPnLPct = pnlPct
		stoploss.PeakPnLPct = pnlPct
		return
	}

	deltaPnL := pnlPct - stoploss.Ascent.LastPnLPct
	unchanged := !stoploss.Ascent.LastTime.IsZero() && pnlPct == stoploss.Ascent.LastPnLPct

	stoploss.CurrentPnLPct = pnlPct

	if pnlPct > stoploss.PeakPnLPct {
		stoploss.PeakPnLPct = pnlPct
	}

	stoploss.State = StoplossArmed
	stoploss.Ascent.Note(pnlPct, now)
	stoploss.ExhaustScore = stoploss.Ascent.Exhaust()
	stoploss.Regime = stoploss.classify()

	if unchanged || stoploss.Ascent.Samples < 2 {
		return
	}

	climb := stoploss.PeakPnLPct - stoploss.ArmPnlPct
	giveback := stoploss.PeakPnLPct - stoploss.CurrentPnLPct
	meanUp := stoploss.Ascent.MeanUp

	if stoploss.Regime == StoplossRegimeClimb && stoploss.Ascent.PeakVelocity > 0 {
		nearPeak := (giveback <= meanUp || giveback == 0) && deltaPnL <= 0
		crestExit := nearPeak && stoploss.ExhaustScore+marketFrictionEvidence >= 1
		health := math.Max(stoploss.ExhaustScore, marketFrictionEvidence)
		trailExit := climb > meanUp && giveback > meanUp &&
			giveback >= climb*(1-health)

		if crestExit || trailExit {
			stoploss.Confirms = 1
			stoploss.State = StoplossTriggered
		}

		return
	}

	// Grind / gap: no positive climb velocity — exit only with market friction.
	if climb > 0 && marketFrictionEvidence > 0 &&
		giveback >= climb*(1-marketFrictionEvidence) {
		stoploss.Confirms = 1
		stoploss.State = StoplossTriggered
	}
}

/*
classify marks arms that never built climb velocity as grind.
*/
func (stoploss *Stoploss) classify() StoplossRegime {
	if stoploss.Ascent.Samples >= 2 && stoploss.Ascent.PeakVelocity <= 0 {
		return StoplossRegimeGrind
	}

	return StoplossRegimeClimb
}

func (stoploss *Stoploss) Triggered() bool {
	return stoploss.State == StoplossTriggered
}

/*
Reset clears a cancelled exit so the position can re-arm from Healthy.
*/
func (stoploss *Stoploss) Reset() {
	pnl := stoploss.CurrentPnLPct
	arm := stoploss.ArmPnlPct
	*stoploss = *NewStoploss(pnl, arm)
	stoploss.Ascent.Seed(pnl, time.Now())
}
