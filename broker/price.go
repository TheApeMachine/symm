package broker

import (
	"context"
	"sync"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type EntryCost struct {
	Total              *decimal.Decimal
	EntryPrice         *decimal.Decimal
	BestAsk            *decimal.Decimal
	BestBid            *decimal.Decimal
	Midpoint           *decimal.Decimal
	GrossNotional      *decimal.Decimal
	EntryFee           *decimal.Decimal
	ExitFeeAtBreakEven *decimal.Decimal
	RoundTripFees      *decimal.Decimal
	BreakEven          *decimal.Decimal
	Spread             *decimal.Decimal
	Impact             *decimal.Decimal
}

type ExecutionSurface struct {
	Symbol          string
	At              time.Time
	SellableQty     *decimal.Decimal
	BookComplete    bool
	BestBid         *decimal.Decimal
	ExecutableQty   *decimal.Decimal
	Gross           *decimal.Decimal
	FullyExecutable bool
	ExecutableVWAP  *decimal.Decimal
	ExecutableValue *decimal.Decimal
}

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
	*runtime.System
	Instrument *Instrument
	Books      BookSource
	api        *websocket.API
	fees       *sync.Map
	tickers    *sync.Map
	normalizer *spot.Normalizer
	anomalies  *AnomalyMonitor
}

// BookSource is the resident book boundary shared by live and captured tapes.
// Both supply SDK books; pricing, fees, sizing and accounting stay on Price.
type BookSource interface {
	Book(string, func(*spotbook.Book))
}

func NewPrice(
	ctx context.Context,
	api *websocket.API,
	instrument *Instrument,
) *Price {
	var normalizer *spot.Normalizer

	if api != nil {
		normalizer = api.Normalizer()
	}

	if normalizer == nil {
		normalizer = spot.NewNormalizer()
	}

	price := &Price{
		System:     runtime.NewSystem(ctx, "price"),
		Instrument: instrument,
		api:        api,
		Books:      api,
		normalizer: normalizer,
		fees:       &sync.Map{},
		tickers:    &sync.Map{},
		anomalies:  NewAnomalyMonitor(ctx, 0),
	}

	if err := errnie.Require(map[string]any{
		"api":        api,
		"instrument": instrument,
	}); err != nil {
		price.Error(err)
		return price
	}

	price.Transition(runtime.WAITING)
	return price
}

func (price *Price) normalize(symbol string) string {
	if price != nil && price.normalizer != nil {
		return price.normalizer.Name(symbol)
	}

	return symbol
}

/* SetFee registers an authoritative fee for a symbol. */
func (price *Price) SetFee(symbol string, fee kraken.TradeVolumeFee) {
	price.fees.Store(price.normalize(symbol), fee)
}

/* Normalizer returns the normalizer used for symbol and precision resolution. */
func (price *Price) Normalizer() *spot.Normalizer {
	if price == nil {
		return nil
	}

	return price.normalizer
}

func (price *Price) Update(ticker *kraken.TickerData) {
	price.tickers.Store(price.normalize(ticker.Symbol), ticker)
}

func (price *Price) Tick(symbol string) *kraken.TickerData {
	value, found := price.tickers.Load(price.normalize(symbol))

	if !found {
		return nil
	}

	return value.(*kraken.TickerData)
}

/* Mark returns the current unit price including the taker fee. */
func (price *Price) Mark(symbol string, side Direction) *decimal.Decimal {
	tick := price.Tick(symbol)

	if tick == nil {
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
func (price *Price) PnL(symbol string, position *Position) *decimal.Decimal {
	exit := price.ExitValue(symbol, position)

	if exit == nil || position == nil {
		return nil
	}

	entryPrice := position.Price()
	entryVolume := position.Volume()

	if entryPrice == nil || entryVolume == nil {
		return nil
	}

	basis := notional(entryPrice, entryVolume)
	return exit.Sub(basis)
}

/* Value reports the net liquidation value of a position. */
func (price *Price) Value(symbol string, position *Position) *decimal.Decimal {
	return price.ExitValue(symbol, position)
}

func (price *Price) ExitValue(symbol string, position *Position) *decimal.Decimal {
	if position == nil {
		return nil
	}

	volume := position.Volume()

	if volume == nil {
		return nil
	}

	tick := price.Tick(symbol)

	if tick == nil || tick.Bid == nil {
		return nil
	}

	return price.WithFee(
		symbol, notional(tick.Bid, volume), SELL,
	)
}

func (price *Price) ReturnPct(symbol string, position *Position) float64 {
	pnl := price.PnL(symbol, position)

	if pnl == nil || position == nil {
		return 0
	}

	entryPrice := position.Price()
	entryVolume := position.Volume()

	if entryPrice == nil || entryVolume == nil {
		return 0
	}

	denom := notional(entryPrice, entryVolume)

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

	if price.Books == nil {
		return nil, errnie.Error(errnie.Err(errnie.NotFound, "price: books unavailable", nil))
	}

	price.Books.Book(symbol, func(book *spotbook.Book) {
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

/* Walk executes a loop over book levels until requested quantity is satisfied. */
func (price *Price) Walk(
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

	/*
		A book thinner than the request is an answer, not a failure. Walk is
		how a caller asks what the displayed book would actually fill, and the
		usual caller is sizing an order against its own cash — so a request
		that runs past the far side is the routine case, not the exceptional
		one. The filled quantity and its cost are returned either way.

		It is reported rather than logged for that reason. Logging it made a
		normal reading about book shape indistinguishable from a fault, at a
		rate of hundreds a second across the instrument universe, which buries
		the faults that do matter.
	*/
	if quantity.Cmp(requested) < 0 {
		return quantity, gross, errnie.Err(
			errnie.UnprocessableContent, "price: insufficient book depth", nil,
		)
	}

	return quantity, gross, nil
}

/* EntryCost prices a complete entry from the resident book and current fee. */
func (price *Price) EntryCost(symbol string, quantity *decimal.Decimal) (*EntryCost, error) {
	if quantity == nil || quantity.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "entry cost: positive quantity required", nil))
	}

	var cost *EntryCost
	var err error

	if price.Books == nil {
		return nil, errnie.Error(errnie.Err(errnie.NotFound, "entry cost: books unavailable", nil))
	}

	price.Books.Book(symbol, func(book *spotbook.Book) {
		if book == nil || book.BestAsk() == nil || book.BestBid() == nil {
			err = errnie.Err(errnie.NotFound, "entry cost: book unavailable for "+symbol, nil)
			return
		}

		/*
			A touch whose sides meet or cross describes no executable entry.
			Like a walk that runs past the far side, this is a reading about
			book shape — the venue quoting both sides from instants that
			disagree — and not a fault in this program. It is reported as
			unprocessable so a caller sizes around it, exactly as it sizes
			around insufficient depth.
		*/
		if book.BestBid().Price.Cmp(book.BestAsk().Price) >= 0 {
			price.recordAnomaly(symbol, AnomalyCrossedBook)
			err = errnie.Err(errnie.UnprocessableContent, "entry cost: crossed book for "+symbol, nil)
			return
		}

		filled, gross, walkErr := price.Walk(book, quantity, BUY)

		if walkErr != nil {
			if errnie.IsUnprocessableContent(walkErr) {
				price.recordAnomaly(symbol, AnomalyInsufficientDepth)
			}

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

		cost = &EntryCost{
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
		price.recordAnomaly(symbol, AnomalyIncompleteBook)
		err = errnie.Err(errnie.UnprocessableContent, "entry cost: complete executable book required for "+symbol, nil)
	}

	return cost, reported(err)
}

/*
reported logs a genuine fault and stays silent about a reading. A book that is
too thin, crossed or incomplete for a requested size is ordinary market shape
and is returned to the caller either way; at the rate the instrument universe
produces those, logging them buries the faults that do matter.
*/
func reported(err error) error {
	if err == nil || errnie.IsUnprocessableContent(err) {
		return err
	}

	return errnie.Error(err)
}

/* SellQuote prices a complete liquidation from visible bids and current fee. */
func (price *Price) SellQuote(symbol string, quantity *decimal.Decimal) (*ExecutionSurface, error) {
	return price.Surface(symbol, quantity, time.Now().UTC())
}

/* Surface prices a complete liquidation; incomplete depth stays explicit. */
func (price *Price) Surface(
	symbol string, quantity *decimal.Decimal, at time.Time,
) (*ExecutionSurface, error) {
	surface := &ExecutionSurface{Symbol: symbol, At: at, SellableQty: quantity}
	var err error

	if price.Books == nil {
		return nil, errnie.Error(errnie.Err(errnie.NotFound, "price: books unavailable", nil))
	}

	price.Books.Book(symbol, func(book *spotbook.Book) {
		if book == nil || book.BestBid() == nil || book.BestAsk() == nil {
			err = errnie.Err(errnie.NotFound, "price: book unavailable for "+symbol, nil)
			return
		}

		// A crossed or touching book prices no liquidation; it reads as
		// book shape, not as a fault. See EntryCost above.
		if book.BestBid().Price.Cmp(book.BestAsk().Price) >= 0 {
			price.recordAnomaly(symbol, AnomalyCrossedBook)
			err = errnie.Err(errnie.UnprocessableContent, "price: crossed book for "+symbol, nil)
			return
		}

		surface.BookComplete = true
		surface.BestBid = book.BestBid().Price
		surface.ExecutableQty = decimalZero

		for bid := book.BestBid(); bid != nil; bid = bid.Lower {
			surface.ExecutableQty = surface.ExecutableQty.Add(bid.Quantity)
		}

		filled, gross, walkErr := price.Walk(book, quantity, SELL)

		if walkErr != nil {
			if errnie.IsUnprocessableContent(walkErr) {
				price.recordAnomaly(symbol, AnomalyInsufficientDepth)
			}

			err = walkErr
			return
		}

		surface.Gross = gross
		surface.FullyExecutable = true
		surface.ExecutableVWAP = gross.Div(filled)
		surface.ExecutableValue = price.WithFee(symbol, gross, SELL)
	})

	return surface, reported(err)
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
	return price.FeeIfAvailable(symbol)
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
	price.Transition(runtime.BUSY)
	result, err := price.api.TradeVolume(symbols)

	if err != nil {
		price.Error(err)
		return errnie.Error(errnie.Err(errnie.IO, "trade volume: failed to fetch", err))
	}

	if result == nil {
		validationErr := errnie.Err(errnie.UnprocessableContent, "trade volume: response required", nil)
		price.Error(validationErr)
		return errnie.Error(validationErr)
	}

	fees := make(map[string]kraken.TradeVolumeFee, len(result.Fees))

	for identifier, fee := range result.Fees {
		if fee.Fee == nil || fee.Fee.Sign() < 0 || fee.Fee.Cmp(decimalHundred) >= 0 {
			validationErr := errnie.Err(errnie.Validation, "trade volume: invalid taker fee for "+identifier, nil)
			price.Error(validationErr)
			return errnie.Error(validationErr)
		}

		fees[price.normalizer.Name(identifier)] = fee
	}

	for _, symbol := range symbols {
		if _, found := fees[price.normalizer.Name(symbol)]; !found {
			notFoundErr := errnie.Err(errnie.NotFound, "trade volume: taker fee missing for "+symbol, nil)
			price.Error(notFoundErr)
			return errnie.Error(notFoundErr)
		}
	}

	for symbol, fee := range fees {
		price.fees.Store(symbol, fee)
	}

	price.Transition(runtime.READY)
	return nil
}

/* WithFee applies the symbol's taker fee in the requested direction. */
func (price *Price) WithFee(
	symbol string,
	amount *decimal.Decimal,
	direction Direction,
) *decimal.Decimal {
	fee := price.Fee(symbol)

	if fee == nil || fee.Fee == nil || amount == nil {
		return nil
	}

	if direction != BUY && direction != SELL {
		return nil
	}

	rate := decimalPercent.Mul(fee.Fee)

	if direction == SELL {
		rate = rate.Mul(decimalNegativeOne)
	}

	scale := max(amount.GetScale(), decimal.DefaultScale)
	return amount.SetScale(scale).OffsetPercent(rate)
}


func (price *Price) recordAnomaly(symbol string, kind AnomalyKind) {
	if price == nil || price.anomalies == nil {
		return
	}

	price.anomalies.Record(price.normalize(symbol), kind)
}

/* Anomalies returns the anomaly monitor recording degraded market shapes. */
func (price *Price) Anomalies() *AnomalyMonitor {
	if price == nil {
		return nil
	}

	return price.anomalies
}

/* MarketHealth returns the continuous health score in [0.0, 1.0] for a symbol. */
func (price *Price) MarketHealth(symbol string) float64 {
	if price == nil || price.anomalies == nil {
		return 1.0
	}

	return price.anomalies.Health(price.normalize(symbol))
}

/* Close releases any resources owned by Price, including its anomaly monitor. */
func (price *Price) Close() error {
	if price == nil || price.anomalies == nil {
		return nil
	}

	return price.anomalies.Close()
}
