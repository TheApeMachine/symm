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

Arms after a small green print, trails a give-back floor, and locks a
take-profit once the peak clears the target — the next down tick settles.
*/
type Stoploss struct {
	State            StoplossState `json:"State"`
	InitialPnLPct    float64       `json:"InitialPnLPct"`
	CurrentPnLPct    float64       `json:"CurrentPnLPct"`
	PreviousPnLPct   float64       `json:"PreviousPnLPct"`
	PeakPnLPct       float64       `json:"PeakPnLPct"`
	FloorPnLPct      float64       `json:"FloorPnLPct"`
	ArmPnlPct        float64       `json:"ArmPnlPct"`
	GivebackPnlPct   float64       `json:"GivebackPnlPct"`
	TakeProfitPnlPct float64       `json:"TakeProfitPnlPct"`
}

/*
Newstoploss initializes a new stoploss with the given PnL percentage.
*/
func NewStoploss(PnLPct float64) *Stoploss {
	stoploss := &Stoploss{
		State:            StoplossHealthy,
		InitialPnLPct:    PnLPct,
		CurrentPnLPct:    PnLPct,
		PeakPnLPct:       PnLPct,
		FloorPnLPct:      PnLPct,
		ArmPnlPct:        0.5,
		TakeProfitPnlPct: 1.0,
		GivebackPnlPct:   0.5,
	}

	return stoploss
}

/*
Update the stoploss state based on the current PnL percentage.
There is no floor to this stoploss, because it has proven to be next to impossible
to deal with the highly chaotic nature of the market. Instead, we rely on the system's
entry guards to make good decisions, and to hold on to positions until they are going
up again. This tracks with long-term observations of the market, where the same symbols
keep repeating the same patterns over and over again.
*/
func (stoploss *Stoploss) Update(PnLPct float64) {
	stoploss.PreviousPnLPct = stoploss.CurrentPnLPct
	stoploss.CurrentPnLPct = PnLPct

	if stoploss.CurrentPnLPct > stoploss.PeakPnLPct {
		stoploss.PeakPnLPct = stoploss.CurrentPnLPct
	}

	if stoploss.CurrentPnLPct > stoploss.ArmPnlPct {
		stoploss.State = StoplossArmed
	}

	if stoploss.CurrentPnLPct > stoploss.TakeProfitPnlPct {
		stoploss.State = StoplossTakeProfit
	}

	if stoploss.State == StoplossArmed {
		stoploss.FloorPnLPct = stoploss.PeakPnLPct * stoploss.GivebackPnlPct
		if stoploss.CurrentPnLPct < stoploss.FloorPnLPct {
			stoploss.State = StoplossTriggered
		}
	}

	if stoploss.State == StoplossTakeProfit && stoploss.CurrentPnLPct < stoploss.PreviousPnLPct {
		stoploss.State = StoplossTriggered
	}
}

/*
Check if the stoploss has been triggered.
*/
func (stoploss *Stoploss) Triggered() bool {
	return stoploss.State == StoplossTriggered
}
