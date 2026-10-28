package broker

import (
	"context"
	"fmt"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

type Direction string

const (
	BUY  Direction = "buy"
	SELL Direction = "sell"
)

var (
	decimalZero        = decimal.NewFromInt64(0)
	decimalOne         = decimal.NewFromInt64(1)
	decimalHundred     = decimal.NewFromInt64(100)
	decimalNegativeOne = decimal.NewFromInt64(-1)
	decimalPercent     = decimalOne.Div(decimalHundred)
)

/*
EntryFill is a complete buy against the ask book at admission time.
*/
type EntryFill struct {
	EntryPrice *decimal.Decimal
	EntryFee   *decimal.Decimal // quote currency paid on entry
}

/*
Price answers four questions about a symbol. Every answer is a fill against
the resident book: a last trade is not a price.

	Fee      taker fee in percent. 0.40 means 0.40%.
	Allocate base quantity a new long may buy.
	PnL      quote profit of selling a long through the bids.
	PnLPct   that profit as a percent of the cash the long cost.
*/
type Price struct {
	*runtime.System
	Instrument *Instrument
	Books      *Book
	balance    *Balance
	private    Transport
	fees       *sync.Map
	normalizer *spot.Normalizer
}

func NewPrice(
	ctx context.Context,
	books *Book,
	private Transport,
	instrument *Instrument,
	normalizer *spot.Normalizer,
	balance *Balance,
) *Price {
	if normalizer == nil {
		normalizer = spot.NewNormalizer()
	}

	price := &Price{
		System:     runtime.NewSystem(ctx, "price"),
		Instrument: instrument,
		private:    private,
		Books:      books,
		balance:    balance,
		normalizer: normalizer,
		fees:       &sync.Map{},
	}

	if !price.loadFees() {
		return nil
	}

	price.Transition(runtime.READY)
	return price
}

func (price *Price) loadFees() bool {
	if price.Instrument == nil {
		price.Error(errnie.Err(
			errnie.Internal, "trade volume fees need instrument symbols", nil,
		))
		return false
	}

	symbols := price.Instrument.Symbols()

	if len(symbols) == 0 {
		price.Error(errnie.Err(
			errnie.Internal, "trade volume fees need a non-empty symbol list", nil,
		))
		return false
	}

	var volume *kraken.TradeVolumeResult

	if paper, ok := price.private.(*Paper); ok {
		result, err := paper.TradeVolume(symbols)

		if err != nil {
			price.Error(err)
			return false
		}

		volume = result
	} else {
		rest := network.NewREST()
		response, err := rest.Post(
			"https://api.kraken.com"+system.Cfg.WebSocket.Endpoints.TradeVolume,
			kraken.NewTradeVolumeRequest(symbols),
		)

		if err != nil {
			price.Error(errnie.Err(
				errnie.IO, "failed to get trade volume fees", err,
			))
			return false
		}

		result, err := kraken.ParseTradeVolume(response.Body())

		if err != nil {
			price.Error(err)
			return false
		}

		volume = result
	}

	stored := 0

	for pair, fee := range volume.Fees {
		if fee.Fee == nil {
			continue
		}

		price.fees.Store(price.normalizer.Name(pair), fee.Fee)
		stored++
	}

	if stored == 0 {
		price.Error(errnie.Err(
			errnie.UnprocessableContent,
			"trade volume returned no usable taker fees",
			nil,
		))
		return false
	}

	return true
}

/*
Fee returns the taker fee for a symbol, in percent. 0.40 means 0.40%.
Nil means the fee is unknown, and every other answer then abstains.
*/
func (price *Price) Fee(symbol string) *decimal.Decimal {
	val, ok := price.fees.Load(price.normalizer.Name(symbol))

	if !ok || val == nil {
		return nil
	}

	return val.(*decimal.Decimal)
}

func (price *Price) Touch(symbol string) (*decimal.Decimal, *decimal.Decimal) {
	if price == nil || price.Books == nil {
		return nil, nil
	}

	var bid, ask *decimal.Decimal

	price.Books.Book(symbol, func(managed *book.Book) {
		if managed == nil {
			return
		}

		if bestBid := managed.BestBid(); bestBid != nil {
			bid = bestBid.Price
		}

		if bestAsk := managed.BestAsk(); bestAsk != nil {
			ask = bestAsk.Price
		}
	})

	return bid, ask
}

func (price *Price) CurrentMark(symbol string) *decimal.Decimal {
	bid, ask := price.Touch(symbol)

	if bid != nil {
		return bid
	}

	return ask
}

/*
ExitRoom is best-bid size divided by quantity: how many times the touch
can absorb the lot (1.0 = once).
*/
func (price *Price) ExitRoom(symbol string, quantity *decimal.Decimal) *decimal.Decimal {
	if price == nil || quantity == nil || quantity.Sign() <= 0 {
		return nil
	}

	touch := price.touchBidQuantity(symbol)
	if touch == nil || touch.Sign() <= 0 {
		return nil
	}

	return touch.Div(quantity)
}

func (price *Price) touchBidQuantity(symbol string) *decimal.Decimal {
	if price == nil || price.Books == nil {
		return nil
	}

	var touch *decimal.Decimal

	price.Books.Book(symbol, func(managed *book.Book) {
		if managed == nil || managed.BestBid() == nil || managed.BestBid().Price == nil {
			return
		}

		bestBid := managed.BestBid()
		if bestBid.Quantity == nil || bestBid.Quantity.Sign() <= 0 {
			return
		}

		// Start with the top bid quantity
		totalDepth := bestBid.Quantity

		// Micro-pennying filter: cluster bids within the immediate touch band
		// (within the spread or pair tick size of the best bid) so a 0.001 bot order
		// does not mask the real wall sitting directly behind it.
		bestPrice := bestBid.Price
		var maxDrop *decimal.Decimal

		if managed.BestAsk() != nil && managed.BestAsk().Price != nil {
			// Tolerance is at most half the bid-ask spread
			spread := managed.BestAsk().Price.Sub(bestPrice)
			if spread.Sign() > 0 {
				maxDrop = spread.Div(decimal.NewFromInt64(2))
			}
		}

		cursor := bestBid.Lower
		for count := 0; count < 3 && cursor != nil && cursor.Price != nil; count++ {
			if cursor.Quantity == nil || cursor.Quantity.Sign() <= 0 {
				cursor = cursor.Lower
				continue
			}

			// If this level is within the immediate touch cluster, include its depth
			drop := bestPrice.Sub(cursor.Price)
			if maxDrop != nil && drop.Cmp(maxDrop) > 0 {
				break
			}

			totalDepth = totalDepth.Add(cursor.Quantity)
			cursor = cursor.Lower
		}

		touch = totalDepth
	})

	return touch
}

/*
spreadFraction is the bid-ask spread as a fraction of mid.
*/
func (price *Price) spreadFraction(symbol string) (*decimal.Decimal, bool) {
	bid, ask := price.Touch(symbol)

	if bid == nil || ask == nil || bid.Sign() <= 0 || ask.Sign() <= 0 || ask.Cmp(bid) <= 0 {
		return nil, false
	}

	mid := bid.Add(ask).Div(decimal.NewFromInt64(2))

	if mid.Sign() <= 0 {
		return nil, false
	}

	return ask.Sub(bid).Div(mid), true
}

/*
SpreadPct is the bid-ask spread in percent of mid, the same units as PnLPct.
*/
func (price *Price) SpreadPct(symbol string) (*decimal.Decimal, bool) {
	spread, ok := price.spreadFraction(symbol)
	if !ok || spread == nil {
		return nil, false
	}

	return spread.Mul(decimalHundred), true
}

/*
Friction is the round-trip cost hurdle: relative spread plus both taker
fee legs, as a fraction of mid (same units as PnL/cost).
*/
func (price *Price) Friction(symbol string) (*decimal.Decimal, bool) {
	spread, ok := price.spreadFraction(symbol)
	if !ok || spread == nil {
		return nil, false
	}

	fee := price.Fee(symbol)

	if fee == nil || fee.Sign() < 0 {
		return nil, false
	}

	twoLegFee := fee.Mul(decimalPercent).Mul(decimal.NewFromInt64(2))
	hurdle := spread.Add(twoLegFee)

	if hurdle.Sign() <= 0 {
		return nil, false
	}

	return hurdle, true
}

/*
OpenDragBudgetPct is the most negative close-now PnLPct still allowed at
entry: three times the round-trip friction hurdle. It is the entry-side
counterpart of the stop's loss budget, which starts at that same friction
hurdle and then widens to the symbol's own worst realized loss.
*/
func (price *Price) OpenDragBudgetPct(symbol string) (*decimal.Decimal, bool) {
	friction, ok := price.Friction(symbol)
	if !ok || friction == nil {
		return nil, false
	}

	budget := friction.Mul(decimal.NewFromInt64(3)).Mul(decimalHundred)
	return budget, true
}

/*
Return is fee-net round-trip profit as a fraction of entry cost.
ok is false when the bid book cannot take the whole quantity.
*/
func (price *Price) Return(
	symbol string,
	quantity, entry, fee *decimal.Decimal,
) (*decimal.Decimal, bool) {
	profit, cost := price.settle(symbol, quantity, entry, fee)

	if profit == nil || cost == nil || cost.Sign() <= 0 {
		return nil, false
	}

	return profit.Div(cost), true
}

/*
PnL is the quote-currency profit of selling quantity through the bid book.

entry is the average price paid per unit of base, before fees. fee is the
taker fee in percent for both legs. A nil fee uses Fee. The result is nil
when the book cannot buy the whole quantity, the touch is crossed, or the
fee is unknown.
*/
func (price *Price) PnL(
	symbol string,
	quantity *decimal.Decimal,
	entry *decimal.Decimal,
	fee *decimal.Decimal,
) *decimal.Decimal {
	profit, _ := price.settle(symbol, quantity, entry, fee)
	return profit
}

/*
PnLPct is PnL as a percent of the cash the position cost to open, fees included.
*/
func (price *Price) PnLPct(
	symbol string,
	quantity *decimal.Decimal,
	entry *decimal.Decimal,
	fee *decimal.Decimal,
) *decimal.Decimal {
	profit, cost := price.settle(symbol, quantity, entry, fee)

	if profit == nil || cost == nil || cost.Sign() <= 0 {
		return nil
	}

	return profit.Div(cost).Mul(decimalHundred)
}

func (price *Price) ExitVWAP(symbol string, quantity *decimal.Decimal) *decimal.Decimal {
	if price == nil || quantity == nil || quantity.Sign() <= 0 {
		return nil
	}

	var vwap *decimal.Decimal

	price.Books.Book(symbol, func(managed *book.Book) {
		if !bookReady(managed) {
			return
		}

		filled, gross, err := price.walk(managed, quantity, SELL)

		if err != nil || filled == nil || gross == nil || filled.Cmp(quantity) < 0 {
			return
		}

		vwap = gross.Div(filled)
	})

	if vwap == nil {
		return nil
	}

	return vwap
}

/*
EntryCost walks the ask book for quantity and returns the fill VWAP and entry fee.
*/
func (price *Price) EntryCost(
	symbol string,
	quantity *decimal.Decimal,
) (*EntryFill, error) {
	if quantity == nil || quantity.Sign() <= 0 {
		return nil, errnie.Err(
			errnie.Validation, "entry cost: positive quantity required", nil,
		)
	}

	if price.Books == nil {
		return nil, errnie.Err(errnie.NotFound, "entry cost: books unavailable", nil)
	}

	fee := price.Fee(symbol)

	if fee == nil {
		return nil, errnie.Err(errnie.NotFound, "entry cost: fee unavailable for "+symbol, nil)
	}

	var fill *EntryFill
	var err error

	price.Books.Book(symbol, func(managed *book.Book) {
		if !bookReady(managed) {
			err = errnie.Err(errnie.NotFound, "entry cost: book unavailable for "+symbol, nil)
			return
		}

		filled, gross, walkErr := price.walk(managed, quantity, BUY)

		if walkErr != nil {
			err = errnie.Err(
				errnie.UnprocessableContent, "entry cost: book cannot fill "+symbol, walkErr,
			)
			return
		}

		total := price.afterFee(gross, fee, BUY)

		if total == nil {
			err = errnie.Err(errnie.NotFound, "entry cost: fee unavailable for "+symbol, nil)
			return
		}

		fill = &EntryFill{
			EntryPrice: gross.Div(filled),
			EntryFee:   total.Sub(gross),
		}
	})

	if fill == nil && err == nil {
		err = errnie.Err(
			errnie.UnprocessableContent,
			"entry cost: complete executable book required for "+symbol,
			nil,
		)
	}

	return fill, err
}

/*
Allocate is the base quantity of a new long and a short reason when nil.

The size is at most one third of the best bid, so an exit is not the whole
touch, and the fee-inclusive cost of lifting the asks is at most
risk.max_fraction of reference cash. The quantity is floored to the venue
increment. It is nil when that size is below the venue minimum, the book
cannot fill it, or the fee or the cash is unknown.
*/
func (price *Price) Allocate(symbol string) (*decimal.Decimal, string) {
	if price == nil {
		return nil, "price is nil"
	}

	if price.Instrument == nil {
		return nil, "instrument snapshot is not loaded"
	}

	if price.Books == nil {
		return nil, "book manager is not wired"
	}

	if price.balance == nil {
		return nil, "balance is not wired"
	}

	pair := price.Instrument.Pair(symbol)
	cash := price.balance.cash(pair.Quote)

	if cash == nil {
		cash = price.balance.cash(price.balance.Quote)
	}

	fee := price.Fee(symbol)

	if cash == nil {
		return nil, fmt.Sprintf(
			"quote cash unknown (pair quote %q, desk quote %q)",
			pair.Quote,
			price.balance.Quote,
		)
	}

	if fee == nil {
		return nil, "taker fee unknown for " + symbol
	}

	if pair.QtyMin == nil || pair.CostMin == nil {
		return nil, "instrument limits missing (QtyMin or CostMin unset)"
	}

	budget := cash.SetScale(int64(pair.PricePrecision)).Mul(system.Cfg.Risk.MaxFraction)

	if budget.Sign() <= 0 {
		return nil, fmt.Sprintf(
			"risk budget is zero (cash=%s max_fraction=%v)",
			cash.String(),
			system.Cfg.Risk.MaxFraction,
		)
	}

	feeRate := fee.SetScale(int64(pair.PricePrecision)).Mul(decimalPercent)
	grossBudget := budget.Div(decimalOne.Add(feeRate))

	var (
		quantity *decimal.Decimal
		reason   string
	)

	price.Books.Book(symbol, func(managed *book.Book) {
		if !bookReady(managed) {
			reason = "level3 book not ready (missing bid/ask or book crossed)"
			return
		}

		touchBid := managed.BestBid().Quantity

		if touchBid == nil || touchBid.Sign() <= 0 {
			reason = "best bid quantity missing or zero"
			return
		}

		touchCap := touchBid.SetScale(int64(pair.QtyPrecision)).Div(system.Cfg.Risk.TouchDivisor)
		qAccum := decimalZero
		remainingGross := grossBudget
		level := managed.BestAsk()

		if level == nil || level.Price == nil {
			reason = "best ask missing"
			return
		}

		maxAsk := level.Price.Mul(decimal.NewFromInt64(2))

		for level != nil && remainingGross.Sign() > 0 {
			if level.Price == nil || level.Quantity == nil ||
				level.Price.Sign() <= 0 || level.Quantity.Sign() <= 0 {
				level = level.Higher
				continue
			}

			if level.Price.Cmp(maxAsk) > 0 {
				break
			}

			levelCost := notional(level.Price, level.Quantity)

			if levelCost.Cmp(remainingGross) <= 0 {
				qAccum = qAccum.Add(level.Quantity)
				remainingGross = remainingGross.Sub(levelCost)
				level = level.Higher
				continue
			}

			qAccum = qAccum.Add(remainingGross.Div(level.Price))
			break
		}

		if qAccum.Cmp(touchCap) > 0 {
			qAccum = touchCap
		}

		if qAccum.Sign() <= 0 {
			reason = fmt.Sprintf(
				"ask walk produced zero size (gross_budget=%s touch_cap=%s)",
				grossBudget.String(),
				touchCap.String(),
			)
			return
		}

		if sized, normErr := price.normalizer.FormatSize(symbol, qAccum); normErr == nil {
			qAccum = sized
		}

		filled, gross, walkErr := price.walk(managed, qAccum, BUY)

		if walkErr != nil {
			reason = "ask walk failed: " + walkErr.Error()
			return
		}

		if filled == nil || gross == nil {
			reason = "ask walk returned no fill"
			return
		}

		total := price.afterFee(gross, fee, BUY)

		for total != nil && total.Cmp(budget) > 0 && filled.Sign() > 0 {
			step := pair.QtyIncrement

			if step == nil || step.Sign() <= 0 {
				step = filled.GetSmallestIncrement()
			}

			if step == nil || step.Sign() <= 0 || filled.Cmp(step) <= 0 {
				reason = fmt.Sprintf(
					"cannot trim fill to fee-inclusive budget %s (filled=%s)",
					budget.String(),
					filled.String(),
				)
				return
			}

			filled = filled.Sub(step)

			var nextWalkErr error
			filled, gross, nextWalkErr = price.walk(managed, filled, BUY)

			if nextWalkErr != nil {
				reason = "budget trim walk failed: " + nextWalkErr.Error()
				return
			}

			total = price.afterFee(gross, fee, BUY)
		}

		if filled.Cmp(pair.QtyMin) < 0 {
			reason = fmt.Sprintf(
				"filled qty below venue minimum (filled=%s qty_min=%s)",
				filled.String(),
				pair.QtyMin.String(),
			)
			return
		}

		if notional(gross.Div(filled), filled).Cmp(pair.CostMin) < 0 {
			reason = fmt.Sprintf(
				"filled notional below venue minimum (cost_min=%s)",
				pair.CostMin.String(),
			)
			return
		}

		quantity = filled
	})

	if quantity != nil {
		return quantity, ""
	}

	if reason == "" {
		reason = "book callback did not size (symbol not in book manager)"
	}

	return nil, reason
}

func (price *Price) settle(
	symbol string,
	quantity, entry, fee *decimal.Decimal,
) (profit, cost *decimal.Decimal) {
	if price == nil || price.Books == nil || quantity == nil || entry == nil ||
		quantity.Sign() <= 0 || entry.Sign() <= 0 {
		return nil, nil
	}

	if fee == nil {
		fee = price.Fee(symbol)
	}

	if fee == nil {
		return nil, nil
	}

	var grossExit *decimal.Decimal

	price.Books.Book(symbol, func(managed *book.Book) {
		if !bookReady(managed) {
			return
		}

		filled, gross, err := price.walk(managed, quantity, SELL)

		if err != nil || filled == nil || gross == nil || filled.Cmp(quantity) < 0 {
			return
		}

		grossExit = gross
	})

	if grossExit == nil {
		return nil, nil
	}

	entryGross := notional(entry, quantity)
	cost = price.afterFee(entryGross, fee, BUY)
	netExit := price.afterFee(grossExit, fee, SELL)

	if cost == nil || netExit == nil {
		return nil, nil
	}

	return netExit.Sub(cost), cost
}

func (price *Price) walk(
	managed *book.Book,
	requested *decimal.Decimal,
	side Direction,
) (*decimal.Decimal, *decimal.Decimal, error) {
	if managed == nil || requested == nil || requested.Sign() <= 0 {
		return nil, nil, errnie.Err(
			errnie.Validation, "[price] valid book and positive quantity required", nil,
		)
	}

	quantity, gross := decimalZero, decimalZero
	remaining := requested
	level := managed.BestBid()

	if side == BUY {
		level = managed.BestAsk()
	}

	for level != nil && remaining.Sign() > 0 {
		next := level.Lower

		if side == BUY {
			next = level.Higher
		}

		fill := level.Quantity

		if fill.Cmp(remaining) > 0 {
			fill = remaining
		}

		gross = gross.Add(notional(level.Price, fill))
		quantity = quantity.Add(fill)
		remaining = remaining.Sub(fill)
		level = next
	}

	if quantity.Cmp(requested) < 0 {
		return quantity, gross, errnie.Err(
			errnie.UnprocessableContent, "[price] insufficient book depth", nil,
		)
	}

	return quantity, gross, nil
}

func (price *Price) afterFee(
	amount *decimal.Decimal,
	fee *decimal.Decimal,
	direction Direction,
) *decimal.Decimal {
	if amount == nil || amount.Sign() <= 0 || fee == nil {
		return nil
	}

	rate := decimalPercent.Mul(fee)

	if direction == SELL {
		rate = rate.Mul(decimalNegativeOne)
	}

	return amount.OffsetPercent(rate)
}

func bookReady(managed *book.Book) bool {
	if managed == nil || managed.BestBid() == nil || managed.BestAsk() == nil {
		return false
	}

	return managed.BestBid().Price.Cmp(managed.BestAsk().Price) < 0
}

func notional(unit, quantity *decimal.Decimal) *decimal.Decimal {
	return unit.SetScale(decimal.DefaultScale).Mul(quantity)
}
