package broker

import (
	"context"
	"fmt"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
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

// defaultTakerFeePercent is used when Kraken trade volume has no fee for a pair (percent: 0.8 = 0.8%).
const defaultTakerFeePercent = 0.8

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
	Tape       *Tape
	Mover      *Mover
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
		Tape:       NewTape(),
		Mover:      NewMover(),
		balance:    balance,
		normalizer: normalizer,
		fees:       &sync.Map{},
	}

	if !price.loadFees() {
		return nil
	}

	price.refreshMovers()
	price.pollMovers()

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

	// Account schedule, paper and live. The paper CLI's fee_rate is the
	// simulator's own number; the desk trades the account's tier.
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

	volume, err := kraken.ParseTradeVolume(response.Body())

	if err != nil {
		price.Error(err)
		return false
	}

	stored := 0

	for pair, fee := range volume.Fees {
		if fee.Fee == nil {
			continue
		}

		name := price.normalizer.Name(pair)
		price.fees.Store(name, fee.Fee)
		price.Instrument.RememberFee(name, fee.Fee)
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
	if price != nil && price.Instrument != nil {
		if fee := price.Instrument.TakerFee(price.feeName(symbol)); fee != nil {
			return fee
		}
	}

	return price.storedFee(price.fees, symbol)
}

func (price *Price) feeForTrade(symbol string) *decimal.Decimal {
	if fee := price.Fee(symbol); fee != nil {
		return fee
	}

	return dFloat(defaultTakerFeePercent)
}

func (price *Price) feeName(symbol string) string {
	if price == nil || price.normalizer == nil {
		return symbol
	}

	return price.normalizer.Name(symbol)
}

func (price *Price) storedFee(table *sync.Map, symbol string) *decimal.Decimal {
	if price == nil || table == nil {
		return nil
	}

	val, ok := table.Load(price.feeName(symbol))

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
			bid = levelPrice(bestBid)
		}

		if bestAsk := managed.BestAsk(); bestAsk != nil {
			ask = levelPrice(bestAsk)
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
		qty := levelQty(bestBid)

		if qty == nil || qty.Sign() <= 0 {
			return
		}

		totalDepth := qty
		bestPrice := levelPrice(bestBid)
		var maxDrop *decimal.Decimal

		if managed.BestAsk() != nil && managed.BestAsk().Price != nil {
			spread := levelPrice(managed.BestAsk()).Sub(bestPrice)

			if spread.Sign() > 0 {
				maxDrop = spread.Div(dInt(2))
			}
		}

		cursor := bestBid.Lower

		for count := 0; count < 3 && cursor != nil && cursor.Price != nil; count++ {
			cursorQty := levelQty(cursor)

			if cursorQty == nil || cursorQty.Sign() <= 0 {
				cursor = cursor.Lower
				continue
			}

			drop := bestPrice.Sub(levelPrice(cursor))

			if maxDrop != nil && drop.Cmp(maxDrop) > 0 {
				break
			}

			totalDepth = totalDepth.Add(cursorQty)
			cursor = cursor.Lower
		}

		touch = totalDepth
	})

	return touch
}

/*
TouchBidCash is the quote notional resting on the top three bid levels.
Used as an absolute liquidity veto so a dead micro-cap book cannot clear entry.
*/
func (price *Price) TouchBidCash(symbol string) (*decimal.Decimal, bool) {
	if price == nil || price.Books == nil || symbol == "" {
		return nil, false
	}

	var cash *decimal.Decimal

	price.Books.Book(symbol, func(managed *book.Book) {
		if managed == nil || managed.BestBid() == nil {
			return
		}

		total := decimalZero
		level := managed.BestBid()

		for count := 0; count < 3 && level != nil; count++ {
			px, qty := levelPrice(level), levelQty(level)

			if px != nil && qty != nil && px.Sign() > 0 && qty.Sign() > 0 {
				total = total.Add(notional(px, qty))
			}

			level = level.Lower
		}

		if total.Sign() > 0 {
			cash = total
		}
	})

	if cash == nil {
		return nil, false
	}

	return cash, true
}

/*
spreadFraction is the bid-ask spread as a fraction of mid.
*/
func (price *Price) spreadFraction(symbol string) (*decimal.Decimal, bool) {
	bid, ask := price.Touch(symbol)

	if bid == nil || ask == nil || bid.Sign() <= 0 || ask.Sign() <= 0 || ask.Cmp(bid) <= 0 {
		return nil, false
	}

	mid := bid.Add(ask).Div(dInt(2))

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

	fee := price.feeForTrade(symbol)

	if fee.Sign() < 0 {
		return nil, false
	}

	twoLegFee := fee.Mul(decimalPercent).Mul(dInt(2))
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

	budget := friction.Mul(dInt(3)).Mul(decimalHundred)
	return budget, true
}

/*
EntryCloseNowPnLPct is the fee-net percent PnL if the desk lot size opened at
the ask walk and closed immediately into the bid book. This is the entry drag
the gate must not exceed for paper or virtual.
*/
func (price *Price) EntryCloseNowPnLPct(symbol string) (*decimal.Decimal, bool) {
	if price == nil {
		return nil, false
	}

	qty, _ := price.Allocate(symbol)
	if qty == nil || qty.Sign() <= 0 {
		return nil, false
	}

	fill, err := price.EntryCost(symbol, qty)
	if err != nil || fill == nil || fill.EntryPrice == nil || fill.EntryPrice.Sign() <= 0 {
		return nil, false
	}

	fee := price.feeForTrade(symbol)
	pct := price.PnLPct(symbol, qty, fill.EntryPrice, fee)
	if pct == nil {
		return nil, false
	}

	return pct, true
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

/*
PnLPctAtSellPrice is net percent PnL if the lot closed at sellPrice, using the
same fee basis as PnL. It does not walk the book; shadow regret on stored ticks
uses the last print as a sell mark.
*/
func (price *Price) PnLPctAtSellPrice(
	symbol string,
	quantity, entry, fee, sellPrice *decimal.Decimal,
) *decimal.Decimal {
	if price == nil || quantity == nil || entry == nil || sellPrice == nil ||
		quantity.Sign() <= 0 || entry.Sign() <= 0 || sellPrice.Sign() <= 0 {
		return nil
	}

	if fee == nil {
		fee = price.feeForTrade(symbol)
	}

	entryGross := notional(entry, quantity)
	cost := price.afterFee(entryGross, fee, BUY)
	grossExit := notional(sellPrice, quantity)
	netExit := price.afterFee(grossExit, fee, SELL)

	if cost == nil || netExit == nil || cost.Sign() <= 0 {
		return nil
	}

	return netExit.Sub(cost).Div(cost).Mul(decimalHundred)
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

	fee := price.feeForTrade(symbol)

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
	return price.allocate(symbol, true)
}

/*
AllocateVirtual sizes a learning lot. Venue qty/cost mins are skipped so a
thin walk can still open an observation sample.
*/
func (price *Price) AllocateVirtual(symbol string) (*decimal.Decimal, string) {
	return price.allocate(symbol, false)
}

func (price *Price) allocate(symbol string, enforceVenueMins bool) (*decimal.Decimal, string) {
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
	cashShop := price.balance.cash(pair.Quote)

	if cashShop == nil {
		cashShop = price.balance.cash(price.balance.Quote)
	}

	fee := price.feeForTrade(symbol)

	if cashShop == nil {
		return nil, fmt.Sprintf(
			"quote cash unknown (pair quote %q, desk quote %q)",
			pair.Quote,
			price.balance.Quote,
		)
	}

	cash := cashShop

	if pair.QtyMin == nil || pair.CostMin == nil {
		return nil, "instrument limits missing (QtyMin or CostMin unset)"
	}

	budget := cash.SetScale(int64(pair.PricePrecision)).Mul(
		dFloat(system.Cfg.Risk.MaxFraction),
	)

	if budget.Sign() <= 0 {
		return nil, fmt.Sprintf(
			"risk budget is zero (cash=%s max_fraction=%v)",
			cash.String(),
			system.Cfg.Risk.MaxFraction,
		)
	}

	feeRate := fee.SetScale(int64(pair.PricePrecision)).Mul(decimalPercent)
	grossBudget := budget.Div(decimalOne.Add(feeRate))

	if why := price.Books.BlockReason(symbol); why != "" {
		return nil, why
	}

	var (
		quantity *decimal.Decimal
		reason   string
	)

	price.Books.Book(symbol, func(managed *book.Book) {
		if !bookReady(managed) {
			reason = "level3 book not ready (missing bid/ask or book crossed)"
			return
		}

		touchBid := levelQty(managed.BestBid())

		if touchBid == nil || touchBid.Sign() <= 0 {
			reason = "best bid quantity missing or zero"
			return
		}

		touchCap := touchBid.SetScale(int64(pair.QtyPrecision)).Div(
			dFloat(system.Cfg.Risk.TouchDivisor),
		)
		qAccum := decimalZero
		remainingGross := grossBudget
		level := managed.BestAsk()

		if level == nil || levelPrice(level) == nil {
			reason = "best ask missing"
			return
		}

		maxAsk := levelPrice(level).Mul(dInt(2))

		for level != nil && remainingGross.Sign() > 0 {
			px, qty := levelPrice(level), levelQty(level)

			if px == nil || qty == nil || px.Sign() <= 0 || qty.Sign() <= 0 {
				level = level.Higher
				continue
			}

			if px.Cmp(maxAsk) > 0 {
				break
			}

			levelCost := notional(px, qty)

			if levelCost.Cmp(remainingGross) <= 0 {
				qAccum = qAccum.Add(qty)
				remainingGross = remainingGross.Sub(levelCost)
				level = level.Higher
				continue
			}

			qAccum = qAccum.Add(remainingGross.Div(px))
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

		if sized, normErr := formatSize(price.normalizer, symbol, qAccum); normErr == nil {
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

		if filled.Sign() <= 0 {
			reason = "ask walk produced non-positive fill"
			return
		}

		underMin := filled.Cmp(pair.QtyMin) < 0 ||
			notional(gross.Div(filled), filled).Cmp(pair.CostMin) < 0

		if underMin {
			bumped, bumpedGross, bumpWhy := price.bumpToVenueMin(
				managed, symbol, pair, fee, budget, touchCap,
			)

			if bumped != nil {
				filled = bumped
				gross = bumpedGross
			} else if enforceVenueMins {
				reason = bumpWhy
				return
			}
		}

		quantity = filled
	})

	if quantity != nil {
		return quantity, ""
	}

	if reason == "" {
		if why := price.Books.BlockReason(symbol); why != "" {
			reason = why
		} else {
			reason = "book callback did not size"
		}
	}

	return nil, reason
}

/*
bumpToVenueMin sizes up to QtyMin (and enough for CostMin) when that lot
fits the fee-inclusive cash budget and stays under the touch-bid carry cap
so an exit is not the whole bid.
*/
func (price *Price) bumpToVenueMin(
	managed *book.Book,
	symbol string,
	pair kraken.InstrumentPair,
	fee, budget, touchCap *decimal.Decimal,
) (filled, gross *decimal.Decimal, why string) {
	if price == nil || managed == nil || pair.QtyMin == nil || pair.CostMin == nil ||
		fee == nil || budget == nil || touchCap == nil {
		return nil, nil, "cannot bump to venue minimum (missing book, limits, or budget)"
	}

	target := pair.QtyMin

	ask := managed.BestAsk()
	askPx := levelPrice(ask)

	if ask == nil || askPx == nil || askPx.Sign() <= 0 {
		return nil, nil, "cannot bump to venue minimum (best ask missing)"
	}

	// CostMin often has a coarse scale; Div would SetScale the ask down to
	// zero and panic. Rebase CostMin onto pair price precision first.
	if pair.CostMin.Sign() > 0 {
		costScale := int64(pair.PricePrecision)

		if costScale < 8 {
			costScale = 8
		}

		costFloor := pair.CostMin.SetScale(costScale).Div(askPx)

		if costFloor.Cmp(target) > 0 {
			target = costFloor
		}
	}

	if sized, err := formatSize(price.normalizer, symbol, target); err == nil {
		target = sized
	}

	if target.Cmp(touchCap) > 0 {
		return nil, nil, fmt.Sprintf(
			"venue minimum %s exceeds touch-bid carry cap %s",
			target.String(),
			touchCap.String(),
		)
	}

	filled, gross, walkErr := price.walk(managed, target, BUY)

	if walkErr != nil || filled == nil || gross == nil {
		why = "ask walk failed sizing venue minimum"
		if walkErr != nil {
			why = why + ": " + walkErr.Error()
		}
		return nil, nil, why
	}

	if filled.Cmp(pair.QtyMin) < 0 {
		return nil, nil, fmt.Sprintf(
			"asks cannot fill venue minimum (filled=%s qty_min=%s)",
			filled.String(),
			pair.QtyMin.String(),
		)
	}

	total := price.afterFee(gross, fee, BUY)

	if total == nil {
		return nil, nil, "venue minimum fee-inclusive cost unknown"
	}

	if total.Cmp(budget) > 0 {
		return nil, nil, fmt.Sprintf(
			"venue minimum costs more than cash budget (need=%s budget=%s)",
			total.String(),
			budget.String(),
		)
	}

	if notional(gross.Div(filled), filled).Cmp(pair.CostMin) < 0 {
		return nil, nil, fmt.Sprintf(
			"venue minimum notional still below cost_min=%s",
			pair.CostMin.String(),
		)
	}

	return filled, gross, ""
}

/*
roundTrip is the fee-net result of buying at entry and selling through the
bid book. entryFee and exitFee are percent rates (0.40 means 0.40%).
*/
func (price *Price) roundTrip(
	symbol string,
	quantity, entry, entryFee, exitFee *decimal.Decimal,
) (profit, cost *decimal.Decimal) {
	if price == nil || price.Books == nil || quantity == nil || entry == nil ||
		quantity.Sign() <= 0 || entry.Sign() <= 0 || entryFee == nil || exitFee == nil {
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

	cost = price.afterFee(notional(entry, quantity), entryFee, BUY)
	netExit := price.afterFee(grossExit, exitFee, SELL)

	if cost == nil || netExit == nil {
		return nil, nil
	}

	return netExit.Sub(cost), cost
}

func (price *Price) roundTripPct(
	symbol string,
	quantity, entry, entryFee, exitFee *decimal.Decimal,
) *decimal.Decimal {
	profit, cost := price.roundTrip(symbol, quantity, entry, entryFee, exitFee)

	if profit == nil || cost == nil || cost.Sign() <= 0 {
		return nil
	}

	return profit.Div(cost).Mul(decimalHundred)
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
		fee = price.feeForTrade(symbol)
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

		fill := levelQty(level)

		if fill == nil {
			level = next
			continue
		}

		if fill.Cmp(remaining) > 0 {
			fill = remaining
		}

		gross = gross.Add(notional(levelPrice(level), fill))
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

/*
BackingRatio is bidCash/askCash inside the configured mid band. ok is false
when the book is not ready or the ask side of the band is empty.
*/
func (price *Price) BackingRatio(symbol string) (ratio float64, ok bool) {
	if price == nil || price.Books == nil || symbol == "" {
		return 0, false
	}

	band := system.Cfg.Risk.BackingBandFrac

	price.Books.Book(symbol, func(managed *book.Book) {
		mid := mustFromSDK(managed.Midpoint())

		if !bookReady(managed) || mid == nil || mid.Sign() <= 0 {
			return
		}

		lo := mid.Mul(dFloat(1 - band))
		hi := mid.Mul(dFloat(1 + band))

		var cash [2]float64

		for idx, start := range []*book.Level{managed.BestBid(), managed.BestAsk()} {
			for lvl := start; lvl != nil; lvl = [...]*book.Level{lvl.Lower, lvl.Higher}[idx] {
				px, qty := levelPrice(lvl), levelQty(lvl)

				if px == nil || qty == nil || px.Sign() <= 0 || qty.Sign() <= 0 {
					continue
				}

				if idx == 0 && px.Cmp(lo) < 0 {
					break
				}

				if idx == 1 && px.Cmp(hi) > 0 {
					break
				}

				cash[idx] += notional(px, qty).MustFloat64()
			}
		}

		if cash[1] > 0 {
			ratio, ok = cash[0]/cash[1], true
		}
	})

	return ratio, ok
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
