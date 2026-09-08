package strategy

import (
	"strings"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

/* LocalLearning owns independent per-symbol virtual experiments on spot Level3. */
type LocalLearning struct {
	journal []hindsight.LearningEvent
	*Knowledge
	Grid                       *learning.Grid
	books                      LearningBook
	price                      *broker.Price
	initial                    *decimal.Decimal
	Record                     func(hindsight.LearningEvent) error
	markets                    map[string]*learningMarket
	now                        func() time.Time
	steps, decisions, resolved uint64
	execution                  *Execution

	/*
		Desk is the set of independent traders learning to recognise a
		development. They share this learner's market view and its books, and
		one memory between them.
	*/
	Desk *Desk
}

/* advance uses one coherent current book for all independent virtual wallets. */
func (local *LocalLearning) advance(message kraken.Level3Data, capture hindsight.CaptureIdentity) error {
	pair := local.price.Instrument.Pair(message.Symbol)

	if pair.Symbol != message.Symbol || !strings.Contains(message.Symbol, "/") {
		return nil
	}
	market := local.markets[message.Symbol]

	if market == nil {
		market = &learningMarket{symbol: message.Symbol, status: "waiting for executable book"}
		local.markets[message.Symbol] = market
	}

	market.at = local.now()
	market.seq, market.capture = capture.Sequence, capture
	regions, version, err := local.Grid.Regions(message.Symbol)

	if err != nil {
		market.status = "waiting for numeric observations"
		return nil
	}

	changed := market.AdvanceImpulse(regions)
	market.gridVersion = version
	market.events = market.events[:0]
	market.status = "waiting for executable book"

	local.books.Book(message.Symbol, func(book *spotbook.Book) {
		if book == nil || book.Bids == nil || book.Asks == nil || book.Bids.High == nil || book.Asks.Low == nil {
			return
		}

		if book.Bids.High.Price.Cmp(book.Asks.Low.Price) >= 0 {
			market.status = "crossed book"
			return
		}

		if len(market.lanes) == 0 {
			err = local.initialize(market)
		}

		if err != nil || len(market.lanes) == 0 {
			return
		}
		market.status = "learning"
		local.measure(market, book)
		err = local.wake(market, book, changed)

		if err != nil {
			return
		}
		err = local.transition(market, book, message.Timestamp, changed)

		if err == nil {
			err = local.execution.Reduce(local, market, book)
		}
	})

	if err != nil {
		return err
	}

	for _, event := range market.events {
		if err := local.Record(event); err != nil {
			return err
		}
	}

	return local.flush()
}

/*
wake hands this instrument's development to the desk when its state has actually
changed, and lets every trader decide what to do about it.

A trader is woken by the market moving, not by the clock. An unchanged state is
the same claim it already answered, so re-asking would fill the record with
repetitions of one decision and let a single moment outvote every other.
*/
func (local *LocalLearning) wake(
	market *learningMarket, book *spotbook.Book, changed bool,
) error {
	if local.Desk == nil || !changed {
		return nil
	}
	development := market.PrecursorContext()

	if len(development) == 0 {
		return nil
	}
	_, err := local.Desk.Observe(
		market.symbol, book, development, market.at, market.seq,
	)

	return err
}

/*
measure folds this book into the instrument's own movement and its own cost.

The round trip is what the venue would actually charge to open and close here:
the taker fee on both legs plus the spread that has to be crossed. Both come
from the same displayed book and fee schedule the wallets execute against, so
the window a decision is measured over is derived from the same economics the
decision itself pays.
*/
func (local *LocalLearning) measure(market *learningMarket, book *spotbook.Book) {
	fee := local.price.FeeIfAvailable(market.symbol)

	if fee == nil || fee.Fee == nil {
		return
	}
	bid, ask := book.BestBid().Price.Float64(), book.BestAsk().Price.Float64()
	mid := (bid + ask) / 2

	if !(mid > 0) {
		return
	}

	// Kraken states the fee as a percentage of notional; both legs pay it.
	roundTrip := 2*fee.Fee.Float64()/100 + (ask-bid)/mid
	market.observe(mid, roundTrip, market.at)
}

/* initialize clones known capital and venue economics, without external flows. */
func (local *LocalLearning) initialize(market *learningMarket) error {
	pair, fee := local.price.Instrument.Pair(market.symbol), local.price.FeeIfAvailable(market.symbol)

	if fee == nil || pair.Symbol == "" || pair.Symbol != market.symbol || !strings.Contains(pair.Symbol, "/") {
		market.status = "waiting for venue economics"
		return nil
	}

	if pair.QtyIncrement == nil || pair.QtyMin == nil || pair.CostMin == nil ||
		pair.QtyIncrement.Sign() <= 0 || pair.QtyMin.Sign() <= 0 || pair.CostMin.Sign() <= 0 ||
		fee.Fee == nil || fee.Fee.Sign() < 0 || fee.Fee.Cmp(decimal.NewFromInt64(100)) >= 0 {
		return errnie.Err(errnie.Validation, "learner: invalid venue rules or fees for "+market.symbol, nil)
	}

	vocabulary := [...]types.Action{types.ActionHold, types.ActionEnter, types.ActionExit, types.ActionScale}
	market.lanes = make([]learningLane, len(vocabulary)+1)

	for index := range market.lanes {
		lane := &market.lanes[index]
		lane.paper = index == len(vocabulary)
		if err := lane.wallet.initialize(local.initial, local.price, market.symbol); err != nil {
			return err
		}
	}

	return nil
}

/*
transition settles due decisions first, then chooses new ones at a changed
impulse. Settlement runs on every book update, so an open position is scored
against fresh executable liquidation prices rather than waiting for a separate
protective mechanism to notice it.
*/
func (local *LocalLearning) transition(
	market *learningMarket,
	book *spotbook.Book,
	marketAt time.Time,
	changed bool,
) error {
	// The instrument's own clock only advances on a real impulse change, so the
	// measured epoch is the cadence of state change rather than of book updates.
	if changed {
		market.epoch(market.at)
	}
	horizon := market.horizon()

	for index := range market.lanes {
		lane := &market.lanes[index]
		hadPending := lane.pending != 0

		if hadPending {
			quantity, gross, fee, err := lane.wallet.fill(book, lane.action, lane.requested)

			if err != nil {
				return err
			}
			event := lane.event(market, index, "filled", lane.pending, marketAt)
			event.Complete = false
			event.Quantity, event.Gross, event.Fee = quantity.String(), gross.String(), fee.String()

			if lane.action.Kind == types.ActionHold {
				event.Kind = "waited"
			}

			if lane.action.Kind != types.ActionHold && quantity.Sign() == 0 {
				event.Kind = "rejected"
			}

			if quantity.Sign() > 0 {
				lane.fills++
			}

			market.events = append(market.events, event)
			lane.pending = 0
		}

		mark, complete, err := lane.wallet.mark(book)

		if err != nil {
			return err
		}
		lane.complete = complete

		if !complete {
			market.status = "open inventory exceeds visible liquidation depth"
			continue
		}

		lane.equity = mark.Float64()
		lane.version++

		if lane.paper {
			market.markExposure(lane.wallet.quantity.Sign() > 0, market.seq, market.at)
		}

		outcome, err := lane.ledger.Measure(EquityMark{
			At: market.at, Version: lane.version, Equity: lane.equity, HasFunding: true,
		})

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[agent] failed to measure lane",
				err,
			))
		}

		lane.outcome = outcome

		if hadPending {
			event := &market.events[len(market.events)-1]
			event.Profit, event.Complete, event.ValuedAt = outcome.TotalReward, true, market.at
		}

		if err := lane.settle(local, market, index, marketAt, horizon); err != nil {
			return err
		}

		if err := lane.recycle(local, market, index, book, marketAt); err != nil {
			return err
		}

		// Keep the intervention fixed while its outcome is being measured.
		// Market valuation and fills still run on every book update.
		if (len(lane.trace) != 0 && !lane.paper) || (!changed && lane.issued != 0) {
			continue
		}

		if len(market.currentConditions) == 0 && lane.wallet.quantity.Sign() == 0 {
			continue
		}

		if err := lane.issue(local, market, index, book, marketAt); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[agent] failed to issue action",
				err,
			))
		}
	}

	return nil
}

/* recordCandidate stages immutable candidate facts until the resident book is released. */
func (local *LocalLearning) recordCandidate(event hindsight.LearningEvent) error {
	local.journal = append(local.journal, event)
	return nil
}

/* flush delivers staged facts in order before an account submission may execute. */
func (local *LocalLearning) flush() error {
	for _, event := range local.journal {
		if err := local.Record(event); err != nil {
			return err
		}
	}
	local.journal = local.journal[:0]
	return nil
}
