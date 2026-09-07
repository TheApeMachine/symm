package broker

import (
	"sync"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
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

/* Price owns fee state and economic calculations using the SDK's decimals. */
type Price struct {
	Instrument *Instrument
	status     types.Status
	api        *websocket.API
	fees       *sync.Map
	tickers    *sync.Map
	normalizer *spot.Normalizer
}

func NewPrice(api *websocket.API, instrument *Instrument) *Price {
	return &Price{
		Instrument: instrument,
		api:        api,
		normalizer: api.Normalizer(),
		fees:       &sync.Map{},
		tickers:    &sync.Map{},
		status:     types.PENDING,
	}
}

func (price *Price) Status() types.Status { return price.status }

func (price *Price) Update(ticker *kraken.TickerData) {
	price.tickers.Store(price.normalizer.Name(ticker.Symbol), ticker)
}

func (price *Price) Tick(symbol string) *kraken.TickerData {
	value, found := price.tickers.Load(price.normalizer.Name(symbol))

	if !found {
		return nil
	}

	return value.(*kraken.TickerData)
}

/* Mark returns the current unit price including the taker fee. */
func (price *Price) Mark(symbol string, side Direction) *decimal.Decimal {
	tick := price.Tick(symbol)

	if tick == nil {
		errnie.Error(errnie.Err(errnie.NotFound, "price: ticker unavailable for "+symbol, nil))
		return nil
	}

	if side == BUY {
		return price.WithFee(symbol, tick.Ask, side)
	}

	return price.WithFee(symbol, tick.Bid, side)
}

/*
notional uses the SDK's working precision for products of differently scaled
price and quantity inputs. Venue order formatting is performed by Normalizer.
*/
func notional(unit, quantity *decimal.Decimal) *decimal.Decimal {
	return unit.SetScale(decimal.DefaultScale).Mul(quantity)
}

/* PnL values the remaining inventory, including its retained entry fee. */
func (price *Price) PnL(symbol string, holding *types.Holding) *decimal.Decimal {
	if holding == nil || holding.Mark == nil || holding.Qty == nil || holding.Basis == nil || holding.EntryFee == nil {
		return nil
	}

	exit := price.ExitValue(symbol, holding)

	if exit == nil {
		return nil
	}

	return exit.Sub(holding.Basis).Sub(holding.EntryFee)
}

/* Value reports the net liquidation value of a holding. */
func (price *Price) Value(symbol string, holding *types.Holding) *decimal.Decimal {
	return price.ExitValue(symbol, holding)
}

func (price *Price) ExitValue(symbol string, holding *types.Holding) *decimal.Decimal {
	if holding == nil || holding.Mark == nil || holding.Qty == nil {
		return nil
	}

	return price.WithFee(
		symbol, notional(holding.Mark, holding.Qty), SELL,
	)
}

func (price *Price) ReturnPct(symbol string, holding *types.Holding) float64 {
	pnl := price.PnL(symbol, holding)

	if pnl == nil || holding.Basis == nil || holding.EntryFee == nil {
		return 0
	}

	denom := holding.Basis.Add(holding.EntryFee)

	if denom.Sign() == 0 {
		return 0
	}

	return pnl.Div(denom).Mul(decimalHundred).Float64()
}

/* Quantity sizes a buy against visible asks and available cash. */
func (price *Price) Quantity(symbol string, cash *decimal.Decimal) (*decimal.Decimal, error) {
	if cash == nil || cash.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "price: positive cash required", nil))
	}

	var quantity *decimal.Decimal
	var err error

	price.api.Book(symbol, func(book *spotbook.Book) {
		if book == nil || book.BestAsk() == nil {
			err = errnie.Err(errnie.NotFound, "price: ask book unavailable for "+symbol, nil)
			return
		}

		quantity, err = price.Affordable(symbol, cash, book.BestAsk().Price)
	})

	if quantity == nil && err == nil {
		err = errnie.Err(errnie.NotFound, "price: book unavailable for "+symbol, nil)
	}

	return quantity, errnie.Error(err)
}

/* Affordable applies cash economics before Kraken's size normalization. */
func (price *Price) Affordable(
	symbol string, cash, unit *decimal.Decimal,
) (*decimal.Decimal, error) {
	cost := price.WithFee(symbol, unit.SetScale(decimal.DefaultScale), BUY)

	if cost == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound, "price: entry fee unavailable for "+symbol, nil,
		))
	}

	requested := cash.SetScale(decimal.DefaultScale).Div(cost)
	quantity, err := price.normalizer.FormatSize(symbol, requested)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"price: cannot normalize quantity for "+symbol,
			err,
		))
	}

	if notional(cost, quantity).Cmp(cash) > 0 {
		quantity = quantity.Sub(quantity.GetSmallestIncrement())
	}

	return quantity, nil
}

/* walk executes a loop over book levels until requested quantity is satisfied. */
func (price *Price) walk(
	book *spotbook.Book,
	requested *decimal.Decimal,
	side Direction,
) (*decimal.Decimal, *decimal.Decimal, error) {
	if book == nil || requested == nil || requested.Sign() <= 0 {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.Validation, "price: valid book and positive quantity required", nil,
		))
	}

	quantity, gross := decimalZero, decimalZero
	remaining := requested
	level := book.BestBid()

	if side == BUY {
		level = book.BestAsk()
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

		cost := notional(level.Price, fill)
		quantity = quantity.Add(fill)
		gross = gross.Add(cost)
		remaining = remaining.Sub(fill)
		level = next
	}

	if quantity.Cmp(requested) < 0 {
		return quantity, gross, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "price: insufficient book depth", nil,
		))
	}

	return quantity, gross, nil
}

/* EntryCost prices a complete entry from the resident book and current fee. */
func (price *Price) EntryCost(symbol string, quantity *decimal.Decimal) (*types.EntryCost, error) {
	if quantity == nil || quantity.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "entry cost: positive quantity required", nil))
	}

	var cost *types.EntryCost
	var err error

	price.api.Book(symbol, func(book *spotbook.Book) {
		if book == nil || book.BestAsk() == nil || book.BestBid() == nil {
			err = errnie.Err(errnie.NotFound, "entry cost: book unavailable for "+symbol, nil)
			return
		}

		if book.BestBid().Price.Cmp(book.BestAsk().Price) >= 0 {
			err = errnie.Err(errnie.Validation, "entry cost: crossed book for "+symbol, nil)
			return
		}

		filled, gross, walkErr := price.walk(book, quantity, BUY)

		if walkErr != nil {
			err = walkErr
			return
		}

		total := price.WithFee(symbol, gross, BUY)
		exitFactor := price.WithFee(symbol, decimalOne, SELL)

		if total == nil || exitFactor == nil {
			err = errnie.Err(errnie.NotFound, "entry cost: fee unavailable for "+symbol, nil)
			return
		}

		entry := gross.Div(filled)
		entryFee := total.Sub(gross)
		breakEvenGross := total.Div(exitFactor)
		exitFee := breakEvenGross.Sub(total)
		midpoint := book.Midpoint()

		cost = &types.EntryCost{
			Total:              total,
			EntryPrice:         entry,
			BestAsk:            book.BestAsk().Price,
			BestBid:            book.BestBid().Price,
			Midpoint:           midpoint,
			GrossNotional:      gross,
			EntryFee:           entryFee,
			ExitFeeAtBreakEven: exitFee,
			RoundTripFees:      entryFee.Add(exitFee),
			BreakEven:          breakEvenGross.Div(filled),
			Spread:             book.BestAsk().Price.Sub(midpoint),
			Impact:             entry.Sub(book.BestAsk().Price),
		}
	})

	if cost == nil && err == nil {
		err = errnie.Err(errnie.UnprocessableContent, "entry cost: complete executable book required for "+symbol, nil)
	}

	return cost, errnie.Error(err)
}

/* SellQuote prices a complete liquidation from visible bids and current fee. */
func (price *Price) SellQuote(symbol string, quantity *decimal.Decimal) (*types.ExecutionSurface, error) {
	return price.Surface(symbol, quantity, time.Now().UTC())
}

/* Surface prices a complete liquidation; incomplete depth stays explicit. */
func (price *Price) Surface(
	symbol string, quantity *decimal.Decimal, at time.Time,
) (*types.ExecutionSurface, error) {
	surface := &types.ExecutionSurface{Symbol: symbol, At: at, SellableQty: quantity}
	var err error

	price.api.Book(symbol, func(book *spotbook.Book) {
		if book == nil || book.BestBid() == nil || book.BestAsk() == nil {
			err = errnie.Err(errnie.NotFound, "price: book unavailable for "+symbol, nil)
			return
		}

		if book.BestBid().Price.Cmp(book.BestAsk().Price) >= 0 {
			err = errnie.Err(errnie.Validation, "price: crossed book for "+symbol, nil)
			return
		}

		surface.BookComplete = true
		surface.BestBid = book.BestBid().Price
		surface.ExecutableQty = decimalZero

		for bid := book.BestBid(); bid != nil; bid = bid.Lower {
			surface.ExecutableQty = surface.ExecutableQty.Add(bid.Quantity)
		}

		filled, gross, walkErr := price.walk(book, quantity, SELL)

		if walkErr != nil {
			err = walkErr
			return
		}

		surface.FullyExecutable = true
		surface.ExecutableVWAP = gross.Div(filled)
		surface.ExecutableValue = price.WithFee(symbol, gross, SELL)
	})

	return surface, errnie.Error(err)
}

/* Tradable checks the instrument's actual quantity and notional minimums. */
func (price *Price) Tradable(symbol string, quantity, unit *decimal.Decimal) bool {
	if quantity == nil || unit == nil || price.Instrument == nil {
		return false
	}

	pair := price.Instrument.Pair(symbol)

	if pair.QtyMin == nil || pair.CostMin == nil {
		return false
	}

	return quantity.Cmp(pair.QtyMin) >= 0 && notional(unit, quantity).Cmp(pair.CostMin) >= 0
}

/* Fee returns the taker fee for a symbol. */
func (price *Price) Fee(symbol string) *kraken.TradeVolumeFee {
	fee := price.FeeIfAvailable(symbol)

	if fee == nil {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"fee not found for "+symbol,
			nil,
		))
	}

	return fee
}

/*
FeeIfAvailable returns the taker fee for a symbol, or nil when the fee
surface has not loaded it yet.
*/
func (price *Price) FeeIfAvailable(symbol string) *kraken.TradeVolumeFee {
	if price == nil {
		return nil
	}

	found, ok := price.fees.Load(price.normalizer.Name(symbol))

	if !ok {
		return nil
	}

	fee, ok := found.(kraken.TradeVolumeFee)

	if !ok {
		return nil
	}

	return &fee
}

/* GetFees normalizes the venue's fee keys once and publishes a complete batch. */
func (price *Price) GetFees(symbols []string) error {
	result, err := price.api.TradeVolume(symbols)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "trade volume: failed to fetch", err))
	}

	if result == nil {
		return errnie.Error(errnie.Err(errnie.UnprocessableContent, "trade volume: response required", nil))
	}

	fees := make(map[string]kraken.TradeVolumeFee, len(result.Fees))

	for identifier, fee := range result.Fees {
		if fee.Fee == nil || fee.Fee.Sign() < 0 || fee.Fee.Cmp(decimalHundred) >= 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "trade volume: invalid taker fee for "+identifier, nil))
		}

		fees[price.normalizer.Name(identifier)] = fee
	}

	for _, symbol := range symbols {
		if _, found := fees[price.normalizer.Name(symbol)]; !found {
			return errnie.Error(errnie.Err(errnie.NotFound, "trade volume: taker fee missing for "+symbol, nil))
		}
	}

	for symbol, fee := range fees {
		price.fees.Store(symbol, fee)
	}

	price.status = types.READY
	return nil
}

/* WithFee applies the symbol's taker fee in the requested direction. */
func (price *Price) WithFee(
	symbol string,
	amount *decimal.Decimal,
	direction Direction,
) *decimal.Decimal {
	fee := price.Fee(symbol)

	if fee == nil || fee.Fee == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"price: taker fee required for fee calculation",
			nil,
		))

		return nil
	}

	if amount == nil {
		errnie.Error(errnie.Err(errnie.Validation, "price: amount required", nil))
		return nil
	}

	if direction != BUY && direction != SELL {
		errnie.Error(errnie.Err(errnie.Validation, "price: buy or sell direction required", nil))
		return nil
	}

	rate := decimalPercent.Mul(fee.Fee)

	if direction == SELL {
		rate = rate.Mul(decimalNegativeOne)
	}

	scale := max(amount.GetScale(), decimal.DefaultScale)
	return amount.SetScale(scale).OffsetPercent(rate)
}

/*
ApplyFill consumes cumulative venue facts. Partial sales allocate finite basis
and fees, retaining the remainder by subtraction; final sales take it all.
*/
func (price *Price) ApplyFill(
	holding *types.Holding,
	execution kraken.ExecutionData,
	previous kraken.ExecutionData,
) error {
	if execution.CumQty == nil || execution.CumQty.Sign() == 0 {
		return nil
	}

	if execution.CumCost == nil || execution.FeeUsdEquiv == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "price: cumulative fill cost and fee required", nil,
		))
	}

	prevQty := decimalZero
	prevCost := decimalZero
	prevFee := decimalZero

	if previous.CumQty != nil {
		prevQty = previous.CumQty
	}

	if previous.CumCost != nil {
		prevCost = previous.CumCost
	}

	if previous.FeeUsdEquiv != nil {
		prevFee = previous.FeeUsdEquiv
	}

	quantity := execution.CumQty.Sub(prevQty)
	cost := execution.CumCost.Sub(prevCost)
	fee := execution.FeeUsdEquiv.Sub(prevFee)

	if quantity.Sign() < 0 || cost.Sign() < 0 || fee.Sign() < 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation, "price: cumulative fill economics moved backwards", nil,
		))
	}

	if holding.Qty == nil {
		holding.Qty = decimalZero
	}

	if holding.Basis == nil {
		holding.Basis = decimalZero
	}

	if holding.EntryCost == nil {
		holding.EntryCost = decimalZero
	}

	if holding.EntryFee == nil {
		holding.EntryFee = decimalZero
	}

	if holding.EntryFees == nil {
		holding.EntryFees = decimalZero
	}

	if holding.EntryQty == nil {
		holding.EntryQty = decimalZero
	}

	if holding.ExitCost == nil {
		holding.ExitCost = decimalZero
	}

	if holding.ExitQty == nil {
		holding.ExitQty = decimalZero
	}

	if holding.ExitFees == nil {
		holding.ExitFees = decimalZero
	}

	if holding.RealizedPnL == nil {
		holding.RealizedPnL = decimalZero
	}

	if execution.Side == "buy" {
		holding.Qty = holding.Qty.Add(quantity)
		holding.SellableQty = holding.Qty
		holding.Basis = holding.Basis.Add(cost)
		holding.EntryCost = holding.EntryCost.Add(cost)
		holding.EntryFee = holding.EntryFee.Add(fee)
		holding.EntryFees = holding.EntryFees.Add(fee)
		holding.EntryQty = holding.EntryQty.Add(quantity)
		holding.EntryPrice = holding.Basis.Div(holding.Qty)
		holding.EntryVWAP = holding.EntryCost.Div(holding.EntryQty)

		if holding.EntryAt == nil {
			holding.EntryAt = &execution.Timestamp
		}

		return nil
	}

	if execution.Side != "sell" || quantity.Cmp(holding.Qty) > 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation, "price: fill side or sold quantity is invalid", nil,
		))
	}

	basis, entryFee := decimalZero, decimalZero

	if quantity.Sign() > 0 {
		basis, entryFee = holding.Basis, holding.EntryFee

		if quantity.Cmp(holding.Qty) < 0 {
			share := quantity.SetScale(decimal.DefaultScale).Div(holding.Qty)
			basis = holding.Basis.Mul(share)
			entryFee = holding.EntryFee.Mul(share)
		}
	}

	holding.Qty = holding.Qty.Sub(quantity)
	holding.SellableQty = holding.Qty
	holding.Basis = holding.Basis.Sub(basis)
	holding.EntryFee = holding.EntryFee.Sub(entryFee)
	holding.ExitCost = holding.ExitCost.Add(cost)
	holding.ExitQty = holding.ExitQty.Add(quantity)
	holding.ExitFees = holding.ExitFees.Add(fee)
	holding.ExitFee = holding.ExitFees
	holding.RealizedPnL = holding.RealizedPnL.Add(
		cost.Sub(fee).Sub(basis).Sub(entryFee),
	)
	holding.ExitPrice = execution.AvgPrice

	if holding.ExitQty.Sign() > 0 {
		holding.ExitVWAP = holding.ExitCost.Div(holding.ExitQty)
	}

	entryTotal := holding.EntryCost.Add(holding.EntryFees)

	if entryTotal.Sign() > 0 {
		holding.RealizedReturn = holding.RealizedPnL.Div(entryTotal)
	}

	if holding.Qty.Sign() == 0 {
		holding.ExitAt = &execution.Timestamp
		holding.PnL = holding.RealizedPnL
	}

	return nil
}
