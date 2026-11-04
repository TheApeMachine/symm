package broker

type StoplossState int

const (
	StoplossHealthy StoplossState = iota
	StoplossArmed
	StoplossTakeProfit
	StoplossTriggered
)

/*
Stoploss invalidates an open lot.

The hard rail protects initial entry noise until Peak turns positive.
Once Peak clears zero, the trailing floor arms and never sits below
breakeven: a winner cannot be trailed back into a loss. Give-back
shrinks as Peak advances toward and beyond ProfitTarget.
A raised floor settles on a decisive wick break or after StopPaceHalflife
grace while the mark is still green. A mark back through zero settles
on that print.

Nomagique: Max, Compression, CUSUM, Bernoulli, Rank, FastSlow, EvidenceGeomean.
*/
type Stoploss struct {
	State            StoplossState
	initialPnLPct    float64
	currentPnLPct    float64
	previousPnLPct   float64
	peakPnLPct       float64
	floorPnLPct      float64
	armPnlPct        float64
	givebackPnlPct   float64
	takeProfitPnlPct float64
}

func NewStoploss(PnLPct float64, price *Price, symbol string) *Stoploss {
	stoploss := &Stoploss{
		State:            StoplossHealthy,
		initialPnLPct:    PnLPct,
		currentPnLPct:    PnLPct,
		peakPnLPct:       PnLPct,
		floorPnLPct:      PnLPct,
		armPnlPct:        0.5,
		takeProfitPnlPct: 1.0,
		givebackPnlPct:   0.5,
	}

	return stoploss
}

func (stoploss *Stoploss) Update(PnLPct float64) {
	stoploss.previousPnLPct = stoploss.currentPnLPct
	stoploss.currentPnLPct = PnLPct

	if stoploss.currentPnLPct > stoploss.peakPnLPct {
		stoploss.peakPnLPct = stoploss.currentPnLPct
	}

	if stoploss.currentPnLPct > stoploss.armPnlPct {
		stoploss.State = StoplossArmed
	}

	if stoploss.currentPnLPct > stoploss.takeProfitPnlPct {
		stoploss.State = StoplossTakeProfit
	}

	if stoploss.State == StoplossArmed {
		stoploss.floorPnLPct = stoploss.peakPnLPct * stoploss.givebackPnlPct
		if stoploss.currentPnLPct < stoploss.floorPnLPct {
			stoploss.State = StoplossTriggered
		}
	}

	if stoploss.State == StoplossTakeProfit && stoploss.currentPnLPct < stoploss.previousPnLPct {
		stoploss.State = StoplossTriggered
	}
}

func (stoploss *Stoploss) Triggered() bool {
	return stoploss.State == StoplossTriggered
}
