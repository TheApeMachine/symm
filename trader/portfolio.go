package trader

import (
	"math"

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
*/
type positionThesis struct {
	entryPrice decimal.Decimal
	entryScore float64
	peakReturn float64
	pending    bool
	exiting    bool
}

/*
Portfolio owns the trade lifecycle. It turns the decision ladder's per-symbol
reads into enter, hold, and exit decisions: a position is opened only when the
symbol is flat and a slot is free, held while its thesis persists, and closed
only when the read reverses (after clearing round-trip friction) or a trailing
stop protects it. The slot budget keeps capital from spreading across churn.
*/
type Portfolio struct {
	theses           map[string]*positionThesis
	recorder         *audit.Recorder
	normalSlots      int
	opportunitySlots int
	trailingOffset   float64
	minOffset        float64
	maxOffset        float64
	friction         float64
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
NewPortfolio builds the lifecycle manager from the trading config, deriving the
minimum-hold friction from the configured fees and slippage rather than a magic
timer.
*/
func NewPortfolio(recorder *audit.Recorder) (*Portfolio, error) {
	portfolio := &Portfolio{
		theses:           map[string]*positionThesis{},
		recorder:         recorder,
		normalSlots:      viper.GetInt("trading.slots.normal"),
		opportunitySlots: viper.GetInt("trading.entry.opportunity_slot_count"),
		trailingOffset:   viper.GetFloat64("trading.stop.trailing_offset_bps") / 10000,
		minOffset:        viper.GetFloat64("trading.stop.min_offset_bps") / 10000,
		maxOffset:        viper.GetFloat64("trading.stop.max_offset_bps") / 10000,
		friction: (2*viper.GetFloat64("trading.paper.taker_fee_bps") +
			2*viper.GetFloat64("trading.paper.slippage_bps")) / 10000,
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

	return portfolio, nil
}

/*
Reconcile folds current holdings and the decision reads into lifecycle commands.
It runs every tick so trailing stops stay live even when no new decision arrives.
*/
func (portfolio *Portfolio) Reconcile(
	actions []*logic.Action,
	holdings map[string]broker.PositionData,
) []tradeIntent {
	portfolio.reconcileState(holdings)

	intents := portfolio.trailingExits(holdings)
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
fill lands, tracks the peak return for the trailing stop, and forgets any thesis
whose position has closed.
*/
func (portfolio *Portfolio) reconcileState(
	holdings map[string]broker.PositionData,
) {
	for symbol, thesis := range portfolio.theses {
		holding, held := holdings[symbol]

		if held {
			thesis.pending = false

			if holding.ReturnPct > thesis.peakReturn {
				thesis.peakReturn = holding.ReturnPct
			}

			continue
		}

		if !thesis.pending {
			delete(portfolio.theses, symbol)
		}
	}
}

/*
trailingExits protects every open position: once return falls a trailing offset
below its peak, the position is cut regardless of the read. This is the only
exit allowed before friction is cleared, so it can stop a loss but cannot churn.
*/
func (portfolio *Portfolio) trailingExits(
	holdings map[string]broker.PositionData,
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

		if holding.ReturnPct > thesis.peakReturn-portfolio.offset() {
			continue
		}

		thesis.exiting = true
		intents = append(intents, tradeIntent{
			kind:   intentExit,
			symbol: symbol,
			reason: "trailing_stop",
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
			if holding.ReturnPct < portfolio.friction {
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
