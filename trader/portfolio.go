package trader

import (
	"math"
	"math/big"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic"
)

const (
	intentEnter = "enter"
	intentExit  = "exit"
)

/*
tradeIntent is a concrete lifecycle command the portfolio wants executed.
*/
type tradeIntent struct {
	kind     string
	symbol   string
	fraction float64
	price    decimal.Decimal
	reason   string
}

/*
positionThesis is the reason a symbol is held and the state needed to decide
when to let it go: the conviction it was opened on, the best return seen so far
(for the trailing stop), and flags tracking an async fill or close in flight.

Stagnation fields track how many times the position has revisited its peak
return zone without breaking significantly higher. When stagnationMaxTouches
is exceeded, stagnationExitPending gives the position one final tick to
break higher before closing — maximizing the chance of exiting near the peak
rather than cutting a breakout that is still igniting.
*/
type positionThesis struct {
	entryPrice            decimal.Decimal
	entryScore            float64
	peakReturn            float64
	peakTouchCount        int
	stagnationExitPending bool
	peakMomentum          float64
	lastMomentum          float64
	momentumSeen          bool
	breakoutHeld          bool
	pending               bool
	exiting               bool
	newPeak               bool
}

/*
Portfolio owns the trade lifecycle. It turns the decision ladder's per-symbol
reads into enter, hold, and exit decisions: a position is opened only when the
symbol is flat and a slot is free, held while its thesis persists, and closed
only when the read reverses (after clearing round-trip friction), a trailing
stop protects it, or stagnation releases the slot.
*/
type Portfolio struct {
	theses                 map[string]*positionThesis
	recorder               *audit.Recorder
	normalSlots            int
	opportunitySlots       int
	trailingOffset         float64
	minOffset              float64
	maxOffset              float64
	momentumDecay          float64
	stagnationMaxTouches   int
	stagnationZoneFraction float64
	takeProfitArm          float64
	takeProfitTightOffset  float64
	takeProfitCap          float64
	breakoutThreshold      float64
	breakoutHoldProb       float64
	traceExits             bool
}

/*
portfolioEvent is one lifecycle record: an enter or exit and why it fired. These
are low frequency (a handful of positions turning over), so they are always
recorded when auditing, unlike the per-measurement decision trace.
*/
type portfolioEvent struct {
	Kind     string          `json:"kind"`
	Symbol   string          `json:"symbol"`
	Reason   string          `json:"reason"`
	Fraction float64         `json:"fraction"`
	Price    decimal.Decimal `json:"price"`
}

/*
exitEvalEvent records why a held position did NOT exit on a given evaluation. It
is the diagnostic for a silent exit funnel: when momentum_decay never fires, this
shows exactly which gate held it — the momentum signal was missing (scored=false),
never ratcheted (peakMomentum=0), the position never cleared friction, or the
momentum simply had not decayed far enough yet. Recorded per held position per
tick only while the audit decision trace is on, since it is high frequency.
*/
type exitEvalEvent struct {
	Symbol               string  `json:"symbol"`
	Blocked              string  `json:"blocked"`
	Scored               bool    `json:"scored"`
	Momentum             float64 `json:"momentum"`
	PeakMomentum         float64 `json:"peakMomentum"`
	DecayFloor           float64 `json:"decayFloor"`
	ReturnPct            float64 `json:"returnPct"`
	Friction             float64 `json:"friction"`
	PeakReturn           float64 `json:"peakReturn"`
	PeakTouchCount       int     `json:"peakTouchCount"`
	StagnationMaxTouches int     `json:"stagnationMaxTouches"`
}

/*
NewPortfolio builds the lifecycle manager from the trading config. The
minimum-hold friction is derived per position from that position's real
round-trip taker fee (from the live Kraken fee schedule) plus the configured
slippage estimate — never a magic timer, and never a faked fee.
*/
func NewPortfolio(recorder *audit.Recorder) (*Portfolio, error) {
	portfolio := &Portfolio{
		theses:                 map[string]*positionThesis{},
		recorder:               recorder,
		normalSlots:            viper.GetInt("trading.slots.normal"),
		opportunitySlots:       viper.GetInt("trading.entry.opportunity_slot_count"),
		trailingOffset:         viper.GetFloat64("trading.stop.trailing_offset_bps") / 10000,
		minOffset:              viper.GetFloat64("trading.stop.min_offset_bps") / 10000,
		maxOffset:              viper.GetFloat64("trading.stop.max_offset_bps") / 10000,
		momentumDecay:          viper.GetFloat64("trading.stop.momentum_decay_fraction"),
		stagnationMaxTouches:   viper.GetInt("trading.stop.stagnation_max_touches"),
		stagnationZoneFraction: viper.GetFloat64("trading.stop.stagnation_zone_fraction"),
		takeProfitArm:          viper.GetFloat64("trading.stop.take_profit_arm_pct"),
		takeProfitTightOffset:  viper.GetFloat64("trading.stop.take_profit_tight_offset_bps") / 10000,
		takeProfitCap:          viper.GetFloat64("trading.stop.take_profit_cap_pct"),
		breakoutThreshold:      viper.GetFloat64("trading.stop.breakout_threshold_pct"),
		breakoutHoldProb:       viper.GetFloat64("trading.stop.breakout_hold_probability"),
		traceExits:             viper.GetBool("system.audit.decisions"),
	}

	if portfolio.normalSlots <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: trading.slots.normal must be positive",
			nil,
		))
	}

	if portfolio.trailingOffset <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: trading.stop.trailing_offset_bps must be positive",
			nil,
		))
	}

	if portfolio.momentumDecay <= 0 || portfolio.momentumDecay >= 1 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: trading.stop.momentum_decay_fraction must be in (0, 1)",
			nil,
		))
	}

	return portfolio, nil
}

/*
frictionFor returns the round-trip cost floor for a held position: entry + exit
taker fees (from the position's real live fee rate) plus entry + exit slippage.
A position must clear this before any profit-taking or reversal exit fires, so it
is never churned out below its actual cost of trading.
*/
func (portfolio *Portfolio) frictionFor(holding broker.PositionData) float64 {
	slippageRat := new(big.Rat)

	if holding.Spread.Rat().Sign() > 0 {
		slippageRat = new(big.Rat).Quo(holding.Spread.Rat(), big.NewRat(2, 1))
	} else {
		markRat := holding.Mark.Rat()

		if markRat.Sign() <= 0 {
			markRat = holding.EntryPrice.Rat()
		}

		if markRat.Sign() > 0 && holding.PriceIncrement.Rat().Sign() > 0 {
			slippageRat = new(big.Rat).Quo(holding.PriceIncrement.Rat(), markRat)
		}
	}

	if slippageRat.Sign() <= 0 {
		slippageRat = big.NewRat(1, 10000)
	}

	feeRateRat := new(big.Rat).SetFloat64(holding.FeeRate)
	frictionRat := new(big.Rat).Add(
		new(big.Rat).Mul(big.NewRat(2, 1), feeRateRat),
		new(big.Rat).Mul(big.NewRat(2, 1), slippageRat),
	)

	friction, _ := frictionRat.Float64()

	return friction
}

/*
Reconcile folds current holdings and the decision reads into lifecycle commands.
It runs every tick so trailing stops, stagnation detection, and momentum-decay
stays live even when no new decision arrives.
*/
func (portfolio *Portfolio) Reconcile(
	actions []*logic.Action,
	holdings map[string]broker.PositionData,
	momentum map[string]float64,
	continuation map[string]float64,
) []tradeIntent {
	portfolio.reconcileState(holdings, momentum)

	intents := portfolio.breakoutExits(holdings, continuation)
	intents = append(intents, portfolio.trailingExits(holdings, momentum)...)
	intents = append(intents, portfolio.stagnationExits(holdings)...)
	intents = append(intents, portfolio.decisionMoves(actions, holdings)...)

	for _, intent := range intents {
		portfolio.record(intent)
	}

	return intents
}

/*
record writes one lifecycle event to the audit recorder. It is a no-op when no
recorder is configured.
*/
func (portfolio *Portfolio) record(intent tradeIntent) {
	if portfolio.recorder == nil {
		return
	}

	if err := audit.Record(portfolio.recorder, "portfolio", portfolioEvent{
		Kind:     intent.kind,
		Symbol:   intent.symbol,
		Reason:   intent.reason,
		Fraction: intent.fraction,
		Price:    intent.price,
	}); err != nil {
		errnie.Error(err)
	}
}

/*
recordExitEval writes one exit-evaluation diagnostic. It is gated on the audit
decision trace because it fires per held position per tick, and is the instrument
for answering "why did momentum_decay not fire" without guessing.
*/
func (portfolio *Portfolio) recordExitEval(event exitEvalEvent) {
	if portfolio.recorder == nil || !portfolio.traceExits {
		return
	}

	if err := audit.Record(portfolio.recorder, "exit_eval", event); err != nil {
		errnie.Error(err)
	}
}

/*
Abort clears the thesis for a symbol whose enter or exit order failed to submit,
so a failed fill never strands a slot or wedges a position in the exiting state.
*/
func (portfolio *Portfolio) Abort(symbol string) {
	thesis, ok := portfolio.theses[symbol]
	if !ok {
		return
	}

	if thesis.pending {
		delete(portfolio.theses, symbol)
		return
	}

	thesis.exiting = false
}

/*
reconcileState syncs bookkeeping with reality: it clears the pending flag once a
fill lands, tracks the peak return for the price trailing stop and the peak field
momentum for the momentum-decay exit, and forgets any thesis whose position has
closed.
*/
func (portfolio *Portfolio) reconcileState(
	holdings map[string]broker.PositionData,
	momentum map[string]float64,
) {
	for symbol, thesis := range portfolio.theses {
		holding, held := holdings[symbol]

		if held {
			thesis.pending = false

			if holding.ReturnPct > thesis.peakReturn {
				if thesis.peakReturn > 0 {
					thesis.newPeak = true
				}

				thesis.peakReturn = holding.ReturnPct
				thesis.peakTouchCount = 0
				thesis.stagnationExitPending = false
			} else {
				thesis.newPeak = false
			}

			// Ratchet the peak field momentum. Unlike price, this is the
			// process energy driving the move, so its peak marks the moment
			// the cascade was strongest — the momentum-decay exit releases
			// the position once energy falls a set fraction below it. Also
			// retain the latest reading so the UI can show live proximity to
			// the decay floor.
			if score, ok := momentum[symbol]; ok {
				thesis.lastMomentum = score
				thesis.momentumSeen = true

				if score > thesis.peakMomentum {
					thesis.peakMomentum = score
				}
			}

			continue
		}

		if !thesis.pending {
			delete(portfolio.theses, symbol)
		}
	}
}

/*
breakoutExits governs a large winner (return past breakoutThreshold) with a
next-tick directional prediction instead of a fixed rule. The continuation signal
is P(price continues up next tick) in [0, 1], blended from the supervised forecast
head, field momentum, and the pump/dump causal read.

  - P(up) >= breakoutHoldProb: the move is predicted to keep running, so hold and
    mark the position as riding on an up-prediction. This is what lets a +12% that
    is about to become +24% keep going rather than being snapped off.
  - P(up) < breakoutHoldProb (but not yet reversed): the edge is gone, so take the
    breakout gain now (reason breakout_take).
  - We were holding on an up-prediction and P(up) has since crossed below 0.5 — the
    prediction was contradicted — while still in profit past friction: exit and
    bank the lesser profit (reason breakout_reversal). This is the "if we are wrong
    about up, take whatever profit remains" protection.

Disabled when breakoutThreshold is 0. Runs before the trailing/momentum stops so a
breakout decision takes precedence over the giveback logic.
*/
func (portfolio *Portfolio) breakoutExits(
	holdings map[string]broker.PositionData,
	continuation map[string]float64,
) []tradeIntent {
	intents := make([]tradeIntent, 0)

	if portfolio.breakoutThreshold <= 0 {
		return intents
	}

	for symbol, thesis := range portfolio.theses {
		if thesis.pending || thesis.exiting {
			continue
		}

		holding, held := holdings[symbol]
		if !held {
			continue
		}

		if holding.ReturnPct < portfolio.breakoutThreshold {
			continue
		}

		probability, scored := continuation[symbol]
		if !scored {
			// No directional read this tick — leave the giveback stops to govern
			// rather than guessing.
			continue
		}

		// Reversal protection: we held on an up-prediction and it has since
		// flipped to down. Bank whatever profit remains above friction.
		if thesis.breakoutHeld && probability < 0.5 {
			if holding.ReturnPct > portfolio.frictionFor(holding) {
				thesis.exiting = true
				intents = append(intents, tradeIntent{
					kind:   intentExit,
					symbol: symbol,
					reason: "breakout_reversal",
				})
			}

			continue
		}

		if probability >= portfolio.breakoutHoldProb {
			// Predicted to keep running — ride it, and remember we are holding on
			// an up-prediction so a later flip triggers the reversal protection.
			thesis.breakoutHeld = true
			continue
		}

		// The directional edge is gone (but not yet reversed): take the breakout
		// gain rather than risk giving it back.
		thesis.exiting = true
		intents = append(intents, tradeIntent{
			kind:   intentExit,
			symbol: symbol,
			reason: "breakout_take",
		})
	}

	return intents
}

/*
trailingExits protects every open position with two complementary giveback
stops. The price trailing stop exits immediately on a deep pullback from the peak
return — the hard safety floor. The momentum-decay stop follows the move up while
the field energy driving it keeps ratcheting, then releases the position once that
energy falls below momentumDecay of its peak — the move has stalled or is dying,
so the slot is freed for a symbol with a fresher push. The momentum stop only
fires once the position has cleared round-trip friction, so a stall always locks a
gain rather than paying fees to churn a flat position out.
*/
func (portfolio *Portfolio) trailingExits(
	holdings map[string]broker.PositionData,
	momentum map[string]float64,
) []tradeIntent {
	intents := make([]tradeIntent, 0)

	for symbol, thesis := range portfolio.theses {
		if thesis.pending || thesis.exiting {
			continue
		}

		holding, held := holdings[symbol]
		if !held {
			continue
		}

		score, scored := momentum[symbol]

		// Diagnostic snapshot of the exit evaluation for this held position.
		// blocked is filled in with the gate that held it (or "fired").
		eval := exitEvalEvent{
			Symbol:               symbol,
			Scored:               scored,
			Momentum:             score,
			PeakMomentum:         thesis.peakMomentum,
			DecayFloor:           thesis.peakMomentum * portfolio.momentumDecay,
			ReturnPct:            holding.ReturnPct,
			Friction:             portfolio.frictionFor(holding),
			PeakReturn:           thesis.peakReturn,
			PeakTouchCount:       thesis.peakTouchCount,
			StagnationMaxTouches: portfolio.stagnationMaxTouches,
		}

		// "This is enough" — absolute backstop. A parabolic winner that reaches
		// the cap is banked outright so it can never fully round-trip.
		if portfolio.takeProfitCap > 0 && holding.ReturnPct >= portfolio.takeProfitCap {
			thesis.exiting = true
			eval.Blocked = "fired_take_profit_cap"
			portfolio.recordExitEval(eval)
			intents = append(intents, tradeIntent{
				kind:   intentExit,
				symbol: symbol,
				reason: "take_profit_cap",
			})

			continue
		}

		// Trailing giveback tightens once the peak has armed the take-profit
		// tier: a large winner keeps running but can only hand back the tight
		// offset before it is banked near the high, instead of the loose ride
		// offset that let big gains round-trip.
		offset := portfolio.offset()
		reason := "trailing_stop"

		if portfolio.takeProfitArm > 0 && thesis.peakReturn >= portfolio.takeProfitArm {
			offset = portfolio.takeProfitTightOffset
			reason = "take_profit_trail"
		}

		if holding.ReturnPct <= thesis.peakReturn-offset {
			thesis.exiting = true
			eval.Blocked = "fired_" + reason
			portfolio.recordExitEval(eval)
			intents = append(intents, tradeIntent{
				kind:   intentExit,
				symbol: symbol,
				reason: reason,
			})

			continue
		}

		// Momentum-decay exit: the move's driving energy has faded below a
		// fraction of its peak. Only act once the position is in profit past
		// friction and a peak was actually observed, so a still-igniting or
		// underwater position is never cut on a transient dip.
		switch {
		case !scored || thesis.peakMomentum <= 0:
			eval.Blocked = "no_momentum_signal"
			portfolio.recordExitEval(eval)
			continue
		case holding.ReturnPct <= portfolio.frictionFor(holding):
			eval.Blocked = "below_friction"
			portfolio.recordExitEval(eval)
			continue
		case score > thesis.peakMomentum*portfolio.momentumDecay:
			eval.Blocked = "momentum_still_high"
			portfolio.recordExitEval(eval)
			continue
		}

		thesis.exiting = true
		eval.Blocked = "fired_momentum_decay"
		portfolio.recordExitEval(eval)
		intents = append(intents, tradeIntent{
			kind:   intentExit,
			symbol: symbol,
			reason: "momentum_decay",
		})
	}

	return intents
}

/*
stagnationExits detects positions that keep revisiting their peak return zone
without breaking higher. Each tick where ReturnPct is within
stagnationZoneFraction of peakReturn counts as a "touch". After
stagnationMaxTouches the position enters a one-tick grace period
(stagnationExitPending) — if the return breaks to a new high on the next tick
the counter resets, giving the breakout a chance to fire. If it does not, the
position exits. This maximizes the chance of exiting near the peak rather than
cutting a breakout that is still igniting within the same tick.

A new peak always resets the touch counter and clears the pending flag, so a
position that keeps making higher highs is never cut for stagnation. The exit
only fires once the position has cleared round-trip friction, so a flat position
is never churned out at a loss. Positions below friction also clear the pending
flag so a dip below costs does not hold the grace period open.
*/
func (portfolio *Portfolio) stagnationExits(
	holdings map[string]broker.PositionData,
) []tradeIntent {
	if portfolio.stagnationMaxTouches <= 0 {
		return nil
	}

	intents := make([]tradeIntent, 0)

	for symbol, thesis := range portfolio.theses {
		if thesis.pending || thesis.exiting {
			continue
		}

		holding, held := holdings[symbol]
		if !held {
			continue
		}

		if holding.ReturnPct <= portfolio.frictionFor(holding) {
			thesis.stagnationExitPending = false
			continue
		}

		if thesis.peakReturn <= 0 {
			continue
		}

		// Check if the current return is within the stagnation zone of the peak.
		// The zone is a fraction of the peak return, so a +1.7% position with
		// zone fraction 0.1 has a zone of 0.17% — any return above 1.53% counts
		// as a touch.
		zone := thesis.peakReturn * portfolio.stagnationZoneFraction
		zoneLower := thesis.peakReturn - zone

		if thesis.newPeak {
			thesis.newPeak = false
			continue
		}

		if holding.ReturnPct >= zoneLower {
			thesis.peakTouchCount++
		} else {
			thesis.stagnationExitPending = false
		}

		if thesis.peakTouchCount < portfolio.stagnationMaxTouches {
			thesis.stagnationExitPending = false
			continue
		}

		// Threshold reached. If we have not yet entered the grace period, set
		// the pending flag and wait one tick — the position may be making an
		// upward move within the same tick that will break to a new high.
		if !thesis.stagnationExitPending {
			thesis.stagnationExitPending = true
			continue
		}

		// Grace period expired. The position had one tick to break higher and
		// did not — close it now.
		thesis.exiting = true
		thesis.stagnationExitPending = false
		intents = append(intents, tradeIntent{
			kind:   intentExit,
			symbol: symbol,
			reason: "stagnation",
		})
	}

	return intents
}

/*
decisionMoves opens on a fresh up-conviction read when flat and slotted, and
closes a held position when the read reverses to down-conviction, but only once
unrealized return has cleared round-trip friction so a reversal exit always locks
a gain rather than paying fees to churn.
*/
func (portfolio *Portfolio) decisionMoves(
	actions []*logic.Action,
	holdings map[string]broker.PositionData,
) []tradeIntent {
	intents := make([]tradeIntent, 0)

	for _, action := range actions {
		thesis := portfolio.theses[action.Symbol]
		holding, held := holdings[action.Symbol]

		if thesis != nil && thesis.exiting {
			continue
		}

		if held && thesis != nil && action.Side == "sell" {
			if holding.ReturnPct < portfolio.frictionFor(holding) {
				continue
			}

			thesis.exiting = true
			intents = append(intents, tradeIntent{
				kind:   intentExit,
				symbol: action.Symbol,
				reason: "thesis_reversal",
			})

			continue
		}

		if held || thesis != nil || action.Side != "buy" {
			continue
		}

		if !portfolio.slotFor(action.Score) {
			continue
		}

		portfolio.theses[action.Symbol] = &positionThesis{
			entryPrice: action.Price,
			entryScore: action.Score,
			pending:    true,
		}
		intents = append(intents, tradeIntent{
			kind:     intentEnter,
			symbol:   action.Symbol,
			fraction: action.Fraction,
			price:    action.Price,
			reason:   "entry",
		})
	}

	return intents
}

/*
slotFor admits an entry into a normal slot while any are free, and into a
reserved opportunity slot only when the read is stronger than the weakest thing
currently held, so the reserve is spent on genuine upgrades, never filler.
*/
func (portfolio *Portfolio) slotFor(score float64) bool {
	active := portfolio.active()

	if active < portfolio.normalSlots {
		return true
	}

	if active < portfolio.normalSlots+portfolio.opportunitySlots {
		return portfolio.opportunity(score)
	}

	return false
}

func (portfolio *Portfolio) active() int {
	count := 0

	for _, thesis := range portfolio.theses {
		if !thesis.exiting {
			count++
		}
	}

	return count
}

func (portfolio *Portfolio) opportunity(score float64) bool {
	weakest := math.Inf(1)

	for _, thesis := range portfolio.theses {
		if thesis.exiting {
			continue
		}

		if thesis.entryScore < weakest {
			weakest = thesis.entryScore
		}
	}

	if math.IsInf(weakest, 1) {
		return true
	}

	return score > weakest
}

func (portfolio *Portfolio) offset() float64 {
	offset := portfolio.trailingOffset

	if offset < portfolio.minOffset {
		offset = portfolio.minOffset
	}

	if offset > portfolio.maxOffset {
		offset = portfolio.maxOffset
	}

	return offset
}

/*
PositionStop is the exit overlay for one open position. It carries all three exits
the UI draws:

  - The price-space trailing stop (StopPrice) — the price at which the trailing
    stop would fire, ratcheting up with peakReturn.
  - The momentum-decay exit, which has no price: the driving energy exits when it
    falls to peakMomentum*momentumDecay. MomentumHealth is that proximity mapped
    to 0..1 (1 = at peak / fully alive, 0 = at the decay floor / exit imminent),
    so the UI can show how close the position is to a momentum exit independently
    of price.
  - The stagnation exit, which has no price: the position exits after revisiting
    its peak return zone stagnationMaxTouches times without breaking higher.
    StagnationHealth is the touch count mapped to 0..1 (1 = just started, 0 = at
    the threshold / exit imminent). StagnationPending indicates the one-tick grace
    period is active — the position will exit next tick if it does not break higher.
*/
type PositionStop struct {
	Symbol               string  `json:"symbol"`
	StopPrice            float64 `json:"stop_price"`
	PeakReturn           float64 `json:"peak_return"`
	StopReturn           float64 `json:"stop_return"`
	Momentum             float64 `json:"momentum"`
	PeakMomentum         float64 `json:"peak_momentum"`
	MomentumFloor        float64 `json:"momentum_floor"`
	MomentumHealth       float64 `json:"momentum_health"`
	MomentumActive       bool    `json:"momentum_active"`
	PeakTouchCount       int     `json:"peak_touch_count"`
	StagnationMaxTouches int     `json:"stagnation_max_touches"`
	StagnationHealth     float64 `json:"stagnation_health"`
	StagnationPending    bool    `json:"stagnation_pending"`
	StagnationActive     bool    `json:"stagnation_active"`
}

/*
Stops returns the trailing-stop overlay for every held position, keyed by symbol.
It reads the live theses the portfolio already owns, so no extra plumbing through
the desk is needed — the trader emits these alongside the position frames.
*/
func (portfolio *Portfolio) Stops() map[string]PositionStop {
	offset := portfolio.offset()
	stops := make(map[string]PositionStop, len(portfolio.theses))

	for symbol, thesis := range portfolio.theses {
		if thesis.pending {
			continue
		}

		entry, _ := thesis.entryPrice.Rat().Float64()
		if entry <= 0 {
			continue
		}

		// The trailing stop fires when ReturnPct <= peakReturn - offset, so the
		// stop price is the entry scaled by that return threshold. Once the peak
		// has armed the take-profit tier the giveback tightens, so the drawn stop
		// line matches the exit that will actually fire.
		effectiveOffset := offset
		if portfolio.takeProfitArm > 0 && thesis.peakReturn >= portfolio.takeProfitArm {
			effectiveOffset = portfolio.takeProfitTightOffset
		}

		stopReturn := thesis.peakReturn - effectiveOffset

		// Momentum-decay proximity: the exit fires at peakMomentum*momentumDecay.
		// Health maps the live momentum from the floor (0) to the peak (1), so
		// the UI shows how much energy is left before a momentum exit.
		floor := thesis.peakMomentum * portfolio.momentumDecay
		health := 1.0

		if span := thesis.peakMomentum - floor; span > 0 {
			health = (thesis.lastMomentum - floor) / span
			health = math.Max(0, math.Min(1, health))
		}

		// Stagnation proximity: the exit fires at stagnationMaxTouches touches.
		// Health maps the current touch count from 0 to maxTouches, so the UI
		// shows how many touches remain before a stagnation exit.
		stagnationHealth := 1.0
		if portfolio.stagnationMaxTouches > 0 {
			stagnationHealth = 1.0 - float64(thesis.peakTouchCount)/float64(portfolio.stagnationMaxTouches)
			stagnationHealth = math.Max(0, math.Min(1, stagnationHealth))
		}

		stops[symbol] = PositionStop{
			Symbol:               symbol,
			StopPrice:            entry * (1 + stopReturn),
			PeakReturn:           thesis.peakReturn,
			StopReturn:           stopReturn,
			Momentum:             thesis.lastMomentum,
			PeakMomentum:         thesis.peakMomentum,
			MomentumFloor:        floor,
			MomentumHealth:       health,
			MomentumActive:       thesis.momentumSeen && thesis.peakMomentum > 0,
			PeakTouchCount:       thesis.peakTouchCount,
			StagnationMaxTouches: portfolio.stagnationMaxTouches,
			StagnationHealth:     stagnationHealth,
			StagnationPending:    thesis.stagnationExitPending,
			StagnationActive:     portfolio.stagnationMaxTouches > 0 && thesis.peakTouchCount > 0,
		}
	}

	return stops
}
