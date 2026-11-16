package broker

import "time"

type StoplossState int

const (
	StoplossHealthy StoplossState = iota
	StoplossArmed
	stoplossLegacyTakeProfit
	StoplossTriggered
)

/*
StoplossRegime is the climb shape inferred from path memory readiness / n1↔n2.
*/
type StoplossRegime string

const (
	StoplossRegimeCrawl    StoplossRegime = "crawl"
	StoplossRegimeIgnition StoplossRegime = "ignition"
	StoplossRegimeSmooth   StoplossRegime = "smooth"
	StoplossRegimeChaos    StoplossRegime = "chaos"
	StoplossRegimeClimb    StoplossRegime = "climb" // legacy alias
	StoplossRegimeGrind    StoplossRegime = "grind" // legacy alias
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
	path          *path          `json:"-"`
}

/*
NewStoploss arms against armPct. Only prints strictly above arm may arm;
never exit at or below arm (profit or hold).
*/
func NewStoploss(pnlPct, armPct float64) *Stoploss {
	return &Stoploss{
		State:         StoplossHealthy,
		Regime:        StoplossRegimeSmooth,
		InitialPnLPct: pnlPct,
		CurrentPnLPct: pnlPct,
		PeakPnLPct:    pnlPct,
		ArmPnlPct:     armPct,
		path:          newPath(),
	}
}

func (stoploss *Stoploss) ensurePath() {
	if stoploss.path == nil {
		stoploss.path = newPath()
	}
}

/*
Update feeds PnL into nomagique path memory and trips only when that memory
says the live print left the learned envelope (Hysteresis-armed). The
exhaustion argument is ignored — cutoffs are derived from path behavior.

Triggered is not sticky underwater: if PnL falls back through the arm before
Exit sends, clear to Healthy so the hub stops painting a frozen crest peak.
*/
func (stoploss *Stoploss) Update(pnlPct float64, _ float64) {
	stoploss.ensurePath()

	if pnlPct <= stoploss.ArmPnlPct {
		stoploss.clearToHealthy(pnlPct)
		return
	}

	if stoploss.State == StoplossTriggered {
		stoploss.CurrentPnLPct = pnlPct
		return
	}

	prev := stoploss.Ascent.LastPnLPct
	had := stoploss.Ascent.Samples > 0
	unchanged := had && pnlPct == prev

	stoploss.CurrentPnLPct = pnlPct

	if stoploss.State == StoplossHealthy || pnlPct > stoploss.PeakPnLPct {
		stoploss.PeakPnLPct = pnlPct
	}

	stoploss.State = StoplossArmed
	reading := stoploss.path.note(pnlPct, time.Now())
	stoploss.Ascent.absorb(reading)
	stoploss.ExhaustScore = stoploss.Ascent.Exhaust()
	stoploss.Regime = stoploss.classify(reading)
	stoploss.PeakPnLPct = reading.peak

	if unchanged || reading.samples < 2 {
		return
	}

	if !reading.armedExit {
		stoploss.Confirms = 0
		return
	}

	stoploss.Confirms++

	if !reading.gateArmed {
		return
	}

	if stoploss.CurrentPnLPct <= stoploss.ArmPnlPct {
		stoploss.Confirms = 0
		return
	}

	stoploss.State = StoplossTriggered
}

/*
classify is inferred from whether down-memory exists and how n2 sits vs n1 —
no fixed exhaust bands.
*/
func (stoploss *Stoploss) classify(reading pathReading) StoplossRegime {
	if reading.ignition {
		return StoplossRegimeIgnition
	}

	n1 := reading.meanUp
	n2 := reading.meanDown

	if n1 > 0 && n2 >= n1 {
		// Chaos when down-drift CUSUM dominates up-drift on a wide n2 path.
		if reading.cusumNeg > reading.cusumPos && n2 > n1 {
			return StoplossRegimeChaos
		}
		return StoplossRegimeCrawl
	}

	return StoplossRegimeSmooth
}

func (stoploss *Stoploss) Triggered() bool {
	return stoploss.State == StoplossTriggered
}

func (stoploss *Stoploss) clearToHealthy(pnlPct float64) {
	stoploss.State = StoplossHealthy
	stoploss.Regime = StoplossRegimeSmooth
	stoploss.path.reset()
	stoploss.Ascent.Reset()
	stoploss.ExhaustScore = 0
	stoploss.Confirms = 0
	stoploss.CurrentPnLPct = pnlPct
	stoploss.PeakPnLPct = pnlPct
}

/*
Reset clears a cancelled exit so the position can re-arm from Healthy.
*/
func (stoploss *Stoploss) Reset() {
	pnl := stoploss.CurrentPnLPct
	arm := stoploss.ArmPnlPct
	*stoploss = *NewStoploss(pnl, arm)
}
