package broker

type StoplossState int

const (
	StoplossHealthy StoplossState = iota
	StoplossArmed
	// stoplossLegacyTakeProfit kept so Triggered stays wire value 3.
	stoplossLegacyTakeProfit
	StoplossTriggered
)

/*
Stoploss invalidates an open lot.

Arms above 1%, trails a keep-ratio floor of peak (50% at arm, +5pp keep per
+1% peak), and never lets the floor sit below 0.5% once armed.
*/
type Stoploss struct {
	State          StoplossState `json:"State"`
	InitialPnLPct  float64       `json:"InitialPnLPct"`
	CurrentPnLPct  float64       `json:"CurrentPnLPct"`
	PreviousPnLPct float64       `json:"PreviousPnLPct"`
	PeakPnLPct     float64       `json:"PeakPnLPct"`
	FloorPnLPct    float64       `json:"FloorPnLPct"`
	ArmPnlPct      float64       `json:"ArmPnlPct"`
	GivebackPnlPct float64       `json:"GivebackPnlPct"`
	FloorMinPnlPct float64       `json:"FloorMinPnlPct"`
	TightenPerPct  float64       `json:"TightenPerPct"`
}

/*
NewStoploss initializes a new stoploss with the given PnL percentage.
*/
func NewStoploss(PnLPct float64) *Stoploss {
	return &Stoploss{
		State:          StoplossHealthy,
		InitialPnLPct:  PnLPct,
		CurrentPnLPct:  PnLPct,
		PeakPnLPct:     PnLPct,
		FloorPnLPct:    PnLPct,
		ArmPnlPct:      1.0,
		GivebackPnlPct: 0.5,
		FloorMinPnlPct: 0.5,
		TightenPerPct:  0.05,
	}
}

/*
Update advances peak, arms above ArmPnlPct, then trails a tightening floor.
*/
func (stoploss *Stoploss) Update(PnLPct float64) {
	stoploss.ensureDefaults()

	stoploss.PreviousPnLPct = stoploss.CurrentPnLPct
	stoploss.CurrentPnLPct = PnLPct

	if stoploss.CurrentPnLPct > stoploss.PeakPnLPct {
		stoploss.PeakPnLPct = stoploss.CurrentPnLPct
	}

	// Legacy TakeProfit wire value folds into Armed with the new arm base.
	if stoploss.State == stoplossLegacyTakeProfit {
		stoploss.State = StoplossArmed
		stoploss.GivebackPnlPct = 0.5
	}

	if stoploss.State == StoplossTriggered {
		return
	}

	if stoploss.State == StoplossHealthy &&
		stoploss.CurrentPnLPct > stoploss.ArmPnlPct {
		stoploss.State = StoplossArmed
	}

	if stoploss.State != StoplossArmed {
		return
	}

	stoploss.trailArmed()
}

/*
ensureDefaults fills trail knobs missing on lots restored before this schema.
*/
func (stoploss *Stoploss) ensureDefaults() {
	if stoploss.ArmPnlPct <= 0 {
		stoploss.ArmPnlPct = 1.0
	}

	if stoploss.GivebackPnlPct <= 0 {
		stoploss.GivebackPnlPct = 0.5
	}

	if stoploss.FloorMinPnlPct <= 0 {
		stoploss.FloorMinPnlPct = 0.5
	}

	if stoploss.TightenPerPct <= 0 {
		stoploss.TightenPerPct = 0.05
	}
}

/*
trailArmed keeps GivebackPnlPct of peak at arm, then raises the keep ratio by
TightenPerPct for each +1% of peak above Arm, never below FloorMinPnlPct.
GivebackPnlPct stays the arm base; the live keep is only folded into Floor.
*/
func (stoploss *Stoploss) trailArmed() {
	above := stoploss.PeakPnLPct - stoploss.ArmPnlPct

	if above < 0 {
		above = 0
	}

	keep := stoploss.GivebackPnlPct + above*stoploss.TightenPerPct

	if keep > 1 {
		keep = 1
	}

	if keep < 0 {
		keep = 0
	}

	floor := stoploss.PeakPnLPct * keep

	if floor < stoploss.FloorMinPnlPct {
		floor = stoploss.FloorMinPnlPct
	}

	stoploss.FloorPnLPct = floor

	if stoploss.CurrentPnLPct < stoploss.FloorPnLPct {
		stoploss.State = StoplossTriggered
	}
}

/*
Triggered reports whether the stoploss has fired.
*/
func (stoploss *Stoploss) Triggered() bool {
	return stoploss.State == StoplossTriggered
}
