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
	StoplossProtected               // Cleared the mirror profit target / locking.
	StoplossTriggered               // Hit the active floor; exit.
)

/*
Stoploss owns invalidation for an open lot.

Hard rails (absolute, centered on 0):
  MaxLossPct      = -MaxAllowedLossPct
  ProfitTargetPct = +MaxAllowedLossPct

Floor starts at MaxLossPct. It breathes from mark Δ, mark pace, and a live
upward probability sensed from the book/tape (backing, spread, 24h change)
plus path momentum (peak vs target).

Damage is peak near-floor proximity in [0,1]. A near-death that later
recovers into the profit target ratchets the floor up.

Once Peak clears ProfitTargetPct, the floor trails by at most MaxAllowedLossPct
below peak (floor >= peak - ProfitTargetPct, and never below 0). At 2× target
or high damage after recovery, the floor ratchets to prior mark so only
flat/up is allowed.
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
	Damage          float64
	UpProb          float64
	PreviousTime    time.Time
	CurrentTime     time.Time
	pace            *adaptive.TimeElastic
	price           *Price
	symbol          string
}

func NewStoploss(PnLPct float64, price *Price, symbol string) *Stoploss {
	maxLoss := system.Cfg.Risk.MaxAllowedLossPct
	now := time.Now()

	stoploss := &Stoploss{
		State:           StoplossStateInit,
		InitialPnLPct:   PnLPct,
		MaxLossPct:      -maxLoss,
		ProfitTargetPct: maxLoss,
		PreviousPnLPct:  PnLPct,
		CurrentPnLPct:   PnLPct,
		PeakPnLPct:      PnLPct,
		FloorPnLPct:     -maxLoss,
		UpProb:          0.5,
		PreviousTime:    now,
		CurrentTime:     now,
		price:           price,
		symbol:          symbol,
	}
	stoploss.rewire()

	return stoploss
}

// rewire restores unexported pace after S3 decode (json skips it).
func (stoploss *Stoploss) rewire() {
	if stoploss == nil || stoploss.pace != nil {
		return
	}

	stoploss.pace = adaptive.NewTimeElastic(adaptive.TimeElasticConfig{
		Halflife: system.Cfg.Risk.StopPaceHalflife,
		Epsilon:  system.Cfg.Risk.StopPaceEpsilon,
	})
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

	stoploss.sense()
	stoploss.scoreDamage(PnLPct)
	stoploss.breathe(PnLPct)
	stoploss.trail()
	stoploss.clampFloor(PnLPct)

	if PnLPct <= stoploss.FloorPnLPct {
		stoploss.State = StoplossTriggered
		return
	}

	if stoploss.PeakPnLPct >= stoploss.ProfitTargetPct {
		stoploss.State = StoplossProtected
		return
	}

	stoploss.State = stoploss.health(PnLPct)
}

func (stoploss *Stoploss) Triggered() bool {
	return stoploss.State == StoplossTriggered
}

/*
Observe rebinds the live book used by sense (also after S3 rewire).
*/
func (stoploss *Stoploss) Observe(price *Price, symbol string) {
	stoploss.price = price
	stoploss.symbol = symbol
}

/*
sense reads book/tape into UpProb ∈ [0,1]. Missing readings stay neutral.
*/
func (stoploss *Stoploss) sense() {
	scores := make([]float64, 0, 4)

	// Path momentum only once the lot has printed green toward the target.
	if stoploss.ProfitTargetPct > 0 && stoploss.PeakPnLPct > 0 {
		scores = append(scores, clamp01(stoploss.PeakPnLPct/stoploss.ProfitTargetPct))
	}

	if stoploss.price == nil || stoploss.symbol == "" {
		stoploss.UpProb = meanOr(scores, 0.5)
		return
	}

	if ratio, ok := stoploss.price.BackingRatio(stoploss.symbol); ok {
		min := system.Cfg.Risk.MinBackingRatio
		if min <= 0 {
			min = 1
		}

		scores = append(scores, clamp01(ratio/(2*min)))
	}

	if spread, ok := stoploss.price.SpreadPct(stoploss.symbol); ok && spread != nil {
		cap := system.Cfg.Risk.MaxEntrySpreadPct
		if cap > 0 {
			scores = append(scores, 1-clamp01(spread.InexactFloat64()/cap))
		}
	}

	change := stoploss.price.ChangePct(stoploss.symbol)
	scores = append(scores, clamp01(0.5+change/200))

	stoploss.UpProb = meanOr(scores, 0.5)
}

/*
scoreDamage raises Damage toward 1 as mark approaches the live floor.
*/
func (stoploss *Stoploss) scoreDamage(pnl float64) {
	span := stoploss.ProfitTargetPct - stoploss.MaxLossPct

	if span <= 0 {
		return
	}

	room := pnl - stoploss.FloorPnLPct
	instant := 1 - clamp01(room/span)

	if instant > stoploss.Damage {
		stoploss.Damage = instant
	}
}

func (stoploss *Stoploss) breathe(pnl float64) {
	delta := pnl - stoploss.PreviousPnLPct

	if delta == 0 {
		stoploss.lockDamage(pnl)
		return
	}

	span := stoploss.ProfitTargetPct - stoploss.MaxLossPct

	if span <= 0 {
		return
	}

	nearFloor := 1 - clamp01((pnl-stoploss.FloorPnLPct)/span)
	nearProfit := 1 - clamp01((stoploss.ProfitTargetPct-pnl)/span)
	pace := stoploss.markPace()
	move := math.Abs(delta) * pace

	if delta > 0 {
		if !stoploss.ratchet() && stoploss.State != StoplossProtected {
			// High UpProb widens more — room for noise while the tape still lifts.
			stoploss.FloorPnLPct -= move * (1 - nearFloor) * stoploss.UpProb
		}

		stoploss.lockDamage(pnl)
		return
	}

	if stoploss.ratchet() {
		// Only flat/up: prior mark becomes the floor.
		if stoploss.PreviousPnLPct > stoploss.FloorPnLPct {
			stoploss.FloorPnLPct = stoploss.PreviousPnLPct
		}

		return
	}

	giveFrac := 0.0

	if stoploss.PeakPnLPct > 0 {
		giveFrac = clamp01((stoploss.PeakPnLPct - pnl) / stoploss.PeakPnLPct)
	}

	// Patience tracks UpProb; fear rises when the book fades, we near rails,
	// we give back peak, or we already took damage.
	fear := (1 - stoploss.UpProb) * math.Max(nearFloor, nearProfit)
	fear = math.Max(fear, stoploss.Damage*nearProfit)

	if stoploss.PeakPnLPct >= stoploss.ProfitTargetPct {
		fear = math.Max(fear, giveFrac)
	}

	stoploss.FloorPnLPct += move * fear
}

/*
lockDamage, after a near-stop recovery into the profit target, lifts the floor
by Damage × ProfitTargetPct so the scare is not forgotten.
*/
func (stoploss *Stoploss) lockDamage(pnl float64) {
	if stoploss.Damage < 0.5 || pnl < stoploss.ProfitTargetPct {
		return
	}

	lock := stoploss.Damage * stoploss.ProfitTargetPct

	if lock > stoploss.FloorPnLPct {
		stoploss.FloorPnLPct = lock
	}
}

/*
trail locks give-back to at most MaxAllowedLossPct below peak once the
mirror profit target has been cleared.
*/
func (stoploss *Stoploss) trail() {
	if stoploss.PeakPnLPct < stoploss.ProfitTargetPct {
		return
	}

	locked := stoploss.PeakPnLPct - stoploss.ProfitTargetPct

	if locked < 0 {
		locked = 0
	}

	if locked > stoploss.FloorPnLPct {
		stoploss.FloorPnLPct = locked
	}
}

func (stoploss *Stoploss) ratchet() bool {
	if stoploss.PeakPnLPct >= 2*stoploss.ProfitTargetPct {
		return true
	}

	return stoploss.Damage >= 0.75 &&
		stoploss.PeakPnLPct >= stoploss.ProfitTargetPct
}

func (stoploss *Stoploss) clampFloor(pnl float64) {
	if stoploss.FloorPnLPct < stoploss.MaxLossPct {
		stoploss.FloorPnLPct = stoploss.MaxLossPct
	}

	if stoploss.PeakPnLPct >= stoploss.ProfitTargetPct && stoploss.FloorPnLPct < 0 {
		stoploss.FloorPnLPct = 0
	}

	// A single mark must not tighten through itself (except ratchet, which
	// deliberately uses the prior mark as the floor).
	if !stoploss.ratchet() && stoploss.FloorPnLPct >= pnl {
		stoploss.FloorPnLPct = math.Nextafter(pnl, math.Inf(-1))
	}

	if stoploss.FloorPnLPct < stoploss.MaxLossPct {
		stoploss.FloorPnLPct = stoploss.MaxLossPct
	}
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

func clamp01(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}

func meanOr(scores []float64, fallback float64) float64 {
	if len(scores) == 0 {
		return fallback
	}

	sum := 0.0

	for _, score := range scores {
		sum += score
	}

	return sum / float64(len(scores))
}
