package strategy

import (
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
	"strings"
	"time"
)

/* LocalLearning owns independent per-symbol virtual experiments on spot Level3. */
type LocalLearning struct {
	journal []hindsight.LearningEvent
	*Knowledge
	Grid                       *learning.Grid
	books                      LearningBook
	pair                       func(string) kraken.InstrumentPair
	fee                        func(string) *kraken.TradeVolumeFee
	initial                    *decimal.Decimal
	Record                     func(hindsight.LearningEvent) error
	markets                    map[string]*learningMarket
	now                        func() time.Time
	steps, decisions, resolved uint64
	execution                  *Execution
}

/* advance uses one coherent current book for all independent virtual wallets. */
func (local *LocalLearning) advance(message kraken.Level3Data, capture hindsight.CaptureIdentity) error {
	pair := local.pair(message.Symbol)

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

	market.regions = append(market.regions[:0], regions...)
	market.context = market.context[:0]
	for _, region := range regions {
		market.context = append(market.context, region.ID)
	}
	changed := market.gridVersion != version
	market.gridVersion = version
	market.sequence = append(market.sequence[:0], market.context...)
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

/* initialize clones known capital and venue economics, without external flows. */
func (local *LocalLearning) initialize(market *learningMarket) error {
	pair, fee := local.pair(market.symbol), local.fee(market.symbol)

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
		lane.wallet.initialize(local.initial, pair, fee.Fee)
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
	if changed {
		market.epoch(market.at)
	}

	for index := range market.lanes {
		lane := &market.lanes[index]
		hadPending := lane.pending != 0

		if hadPending {
			quantity, gross, fee := lane.wallet.fill(book, lane.action, lane.requested, &lane.ladder)
			event := lane.event(market, index, "filled", lane.pending, marketAt)
			event.Complete = false
			event.Quantity, event.Gross, event.Fee = quantity.FloatString(lane.wallet.pair.QtyPrecision), gross.FloatString(lane.wallet.scale), fee.FloatString(lane.wallet.scale)

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

		mark, complete := lane.wallet.mark(book)
		lane.complete = complete

		if !complete {
			market.status = "open inventory exceeds visible liquidation depth"
			continue
		}

		lane.equity, _ = mark.Float64()
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

		if err := lane.settle(local, market, index, marketAt, market.horizon()); err != nil {
			return err
		}

		if err := lane.recycle(local, market, index, book, marketAt); err != nil {
			return err
		}

		if !changed && lane.issued != 0 {
			continue
		}

		if len(market.sequence) == 0 && lane.wallet.quantity.Sign() == 0 {
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
