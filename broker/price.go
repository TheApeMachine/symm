package broker

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
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

/*
EntryCost is the current, observable execution boundary for one proposed long.
It states only facts available at admission time: visible entry VWAP, the
crossing costs paid now, and the sale price that would recover both known fees.
It deliberately contains no future price, future spread, or expected return.
*/
type EntryCost struct {
	Total              *decimal.Decimal `json:"total,omitempty"` // Gross notional plus the entry fee.
	Quantity           *decimal.Decimal `json:"quantity,omitempty"`
	EntryPrice         *decimal.Decimal `json:"entryPrice,omitempty"`
	BestAsk            *decimal.Decimal `json:"bestAsk,omitempty"`
	BestBid            *decimal.Decimal `json:"bestBid,omitempty"`
	Midpoint           *decimal.Decimal `json:"midpoint,omitempty"`
	GrossNotional      *decimal.Decimal `json:"grossNotional,omitempty"`
	EntryFee           *decimal.Decimal `json:"entryFee,omitempty"`
	ExitFeeAtBreakEven *decimal.Decimal `json:"exitFeeAtBreakEven,omitempty"`
	RoundTripFees      *decimal.Decimal `json:"roundTripFees,omitempty"`
	Spread             *decimal.Decimal `json:"spread,omitempty"`
	Impact             *decimal.Decimal `json:"impact,omitempty"`
	BreakEven          *decimal.Decimal `json:"breakEven,omitempty"`
}

/* Price owns fee state and economic calculations using the SDK's decimals. */
type BookSource interface {
	Book(string, func(*spotbook.Book))
}

type Price struct {
	*runtime.System
	Instrument    *Instrument
	Books         BookSource
	private       Transport
	fees          *sync.Map
	tickers       *sync.Map
	normalizer    *spot.Normalizer
	anomalies     *AnomalyMonitor
	referenceCash atomic.Pointer[decimal.Decimal]
}

// BookSource is the resident book boundary shared by live and captured tapes.
// Both supply SDK books; pricing, fees, sizing and accounting stay on Price.

func NewPrice(
	ctx context.Context,
	books BookSource,
	private Transport,
	instrument *Instrument,
	normalizer *spot.Normalizer,
) *Price {
	if normalizer == nil {
		// Tests and callers that never size orders may omit the seeded map.
		// Live trading must pass a Normalizer loaded via Use or Update.
		normalizer = spot.NewNormalizer()
	}

	price := &Price{
		System:     runtime.NewSystem(ctx, "price"),
		Instrument: instrument,
		private:    private,
		Books:      books,
		normalizer: normalizer,
		fees:       &sync.Map{},
		tickers:    &sync.Map{},
		anomalies:  NewAnomalyMonitor(ctx, 0),
	}

	price.anomalies.SetOnFault(func(symbol string) {
		price.Transition(runtime.ERROR)
	})

	price.anomalies.SetOnRecover(func(symbol string) {
		if !price.anomalies.HasAnySevereFault() {
			price.Transition(runtime.READY)
		}
	})

	if err := errnie.Require(map[string]any{
		"private":    private,
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

/* CopyFeesTo transfers all configured symbol fees to destination price system. */
func (price *Price) CopyFeesTo(dst *Price) {
	if price == nil || dst == nil || price.fees == nil {
		return
	}

	price.fees.Range(func(key, value any) bool {
		dst.fees.Store(key, value)
		return true
	})
}

func (price *Price) SetReferenceCash(cash *decimal.Decimal) {
	if price == nil || cash == nil {
		return
	}

	price.referenceCash.Store(cash)
}

func (price *Price) ReferenceCash() *decimal.Decimal {
	if price == nil {
		return nil
	}

	return price.referenceCash.Load()
}

/* Normalizer returns the normalizer used for symbol and precision resolution. */
func (price *Price) Normalizer() *spot.Normalizer {
	if price == nil {
		return nil
	}

	return price.normalizer
}

/*
SeedNormalizer loads public Assets and AssetPairs into n so FormatSize/FormatPrice
and Name resolve venue lot facts. Call once at construction; do not invent scales.
*/
func SeedNormalizer(n *spot.Normalizer) error {
	if n == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"price: normalizer required for asset pair seeding",
			nil,
		))
	}

	if err := n.Use(spot.NewREST()); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"price: failed to load asset pair normalizer from public REST",
			err,
		))
	}

	return nil
}

func (price *Price) Update(ticker *kraken.TickerData) {
	if ticker == nil {
		return
	}

	normalized := price.normalize(ticker.Symbol)
	price.tickers.Store(normalized, ticker)

	if ticker.Bid != nil && ticker.Ask != nil {
		if ticker.Bid.Cmp(ticker.Ask) >= 0 {
			price.recordAnomaly(normalized, AnomalyCrossedBook)
			return
		}

		price.recordClean(normalized)
	}
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
func (price *Price) PnL(symbol string, reg *position.Regulator) *decimal.Decimal {
	if price == nil || reg == nil || !reg.IsHolding() {
		return nil
	}

	exit := price.ExitValue(symbol, reg)

	if exit == nil || reg.Basis == nil {
		return nil
	}

	return exit.Sub(reg.Basis)
}

/* CurrentMark returns the best observable reference price for a symbol. */
func (price *Price) CurrentMark(symbol string) *decimal.Decimal {
	if price == nil {
		return nil
	}

	tick := price.Tick(symbol)

	if tick != nil && tick.Bid != nil {
		return tick.Bid
	}

	if tick != nil && tick.Ask != nil {
		return tick.Ask
	}

	var bookMark *decimal.Decimal

	if price.Books != nil {
		price.Books.Book(symbol, func(managedBook *spotbook.Book) {
			if managedBook == nil {
				return
			}

			if bestBid := managedBook.BestBid(); bestBid != nil {
				bookMark = bestBid.Price
				return
			}

			if bestAsk := managedBook.BestAsk(); bestAsk != nil {
				bookMark = bestAsk.Price
			}
		})
	}

	return bookMark
}

/* Value reports the net liquidation value of a position. */
func (price *Price) Value(symbol string, reg *position.Regulator) *decimal.Decimal {
	return price.ExitValue(symbol, reg)
}

func (price *Price) ExitValue(symbol string, reg *position.Regulator) *decimal.Decimal {
	if price == nil || reg == nil || !reg.IsHolding() {
		return nil
	}

	volume := reg.Volume()

	if volume == nil || volume.Sign() <= 0 {
		return nil
	}

	mark := price.CurrentMark(symbol)

	if mark == nil {
		return nil
	}

	return price.WithFee(
		symbol, notional(mark, volume), SELL,
	)
}

func (price *Price) ReturnPct(symbol string, reg *position.Regulator) float64 {
	pnl := price.PnL(symbol, reg)

	if pnl == nil || reg == nil || reg.Basis == nil || reg.Basis.Sign() <= 0 {
		return 0
	}

	return pnl.Div(reg.Basis).Mul(decimalHundred).Float64()
}

/* Quantity sizes a buy against visible asks and available cash. */
func (price *Price) Quantity(symbol string, cash *decimal.Decimal) (*decimal.Decimal, error) {
	if cash == nil || cash.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "price: positive cash required", nil))
	}

	var quantity *decimal.Decimal
	var unit *decimal.Decimal
	var err error

	if price.Books != nil {
		price.Books.Book(symbol, func(managedBook *spotbook.Book) {
			if managedBook != nil && managedBook.BestAsk() != nil {
				unit = managedBook.BestAsk().Price
				quantity, err = price.Affordable(symbol, cash, unit)
			}
		})
	}

	if quantity == nil {
		if tick := price.Tick(symbol); tick != nil && tick.Ask != nil {
			unit = tick.Ask
			quantity, err = price.Affordable(symbol, cash, unit)
		}
	}

	if quantity == nil && err == nil {
		err = errnie.Err(errnie.NotFound, "price: book unavailable for "+symbol, nil)
	}

	// Cash too small for QtyMin/CostMin → abstain, do not round up into an
	// unaffordable venue order (paper CLI: Insufficient USD Available).
	if err == nil && quantity != nil && unit != nil && price.Instrument != nil {
		if !price.Tradable(symbol, quantity, unit) {
			err = errnie.Err(
				errnie.Validation,
				"price: available cash below minimum order for "+symbol,
				nil,
			)
			quantity = nil
		}
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

		price.recordClean(symbol)

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
			Quantity:           filled,
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

		err = errnie.Err(
			errnie.UnprocessableContent,
			"entry cost: complete executable book required for "+symbol,
			nil,
		)
	}

	return cost, reported(err)
}

/*
AllocateEntry prices an executable entry frozen at 20% of reference cash.
It walks ask depth to derive the maximum executable quantity such that
gross notional plus taker entry fee does not exceed the 20% virtual budget.
*/
func (price *Price) AllocateEntry(
	symbol string,
	referenceCash ...*decimal.Decimal,
) (*EntryCost, error) {
	if price == nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "price: price system required", nil))
	}

	var cash *decimal.Decimal

	if len(referenceCash) > 0 && referenceCash[0] != nil && referenceCash[0].Sign() > 0 {
		cash = referenceCash[0]
	}

	if cash == nil {
		cash = price.ReferenceCash()
	}

	if cash == nil || cash.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"price: positive reference cash required to allocate entry",
			nil,
		))
	}

	// 20% virtual position allocation frozen at entry.
	budget := cash.SetScale(decimal.DefaultScale).Div(decimal.NewFromInt64(5))
	fee := price.Fee(symbol)

	if fee == nil || fee.Fee == nil {
		return nil, errnie.Error(errnie.Err(errnie.NotFound, "price: fee unavailable for "+symbol, nil))
	}

	feeRate := decimalPercent.Mul(fee.Fee)
	maxGross := budget.Div(decimalOne.Add(feeRate))

	var cost *EntryCost
	var err error

	if price.Books != nil {
		price.Books.Book(symbol, func(book *spotbook.Book) {
			if book == nil || book.BestAsk() == nil || book.BestBid() == nil {
				err = errnie.Err(errnie.NotFound, "price: book unavailable for "+symbol, nil)
				return
			}

			if book.BestBid().Price.Cmp(book.BestAsk().Price) >= 0 {
				price.recordAnomaly(symbol, AnomalyCrossedBook)
				err = errnie.Err(errnie.UnprocessableContent, "price: crossed book for "+symbol, nil)
				return
			}

			price.recordClean(symbol)

			qAccum := decimalZero
			remainingGross := maxGross
			level := book.BestAsk()

			for level != nil && remainingGross.Sign() > 0 {
				levelCost := notional(level.Price, level.Quantity)

				if levelCost.Cmp(remainingGross) <= 0 {
					qAccum = qAccum.Add(level.Quantity)
					remainingGross = remainingGross.Sub(levelCost)
					level = level.Higher
					continue
				}

				partialQty := remainingGross.Div(level.Price)
				qAccum = qAccum.Add(partialQty)
				break
			}

			if qAccum.Sign() <= 0 {
				err = errnie.Err(errnie.Validation, "price: budget insufficient to fill any ask depth for "+symbol, nil)
				return
			}

			qNorm, normErr := price.normalizer.FormatSize(symbol, qAccum)

			if normErr != nil {
				qNorm = qAccum
			}

			filled, gross, walkErr := price.Walk(book, qNorm, BUY)

			if walkErr != nil && !errnie.IsUnprocessableContent(walkErr) {
				err = walkErr
				return
			}

			total := price.WithFee(symbol, gross, BUY)

			for total != nil && total.Cmp(budget) > 0 {
				step := qNorm.GetSmallestIncrement()

				if step == nil || step.Sign() <= 0 {
					step = decimal.NewFromFloat64(0.0001)
				}

				if qNorm.Cmp(step) <= 0 {
					err = errnie.Err(errnie.Validation, "price: budget insufficient for minimum tradable quantity for "+symbol, nil)
					return
				}

				qNorm = qNorm.Sub(step)
				filled, gross, walkErr = price.Walk(book, qNorm, BUY)

				if walkErr != nil && !errnie.IsUnprocessableContent(walkErr) {
					err = walkErr
					return
				}

				total = price.WithFee(symbol, gross, BUY)
			}

			if filled == nil || filled.Sign() <= 0 || gross == nil || gross.Sign() <= 0 || total == nil {
				err = errnie.Err(errnie.Validation, "price: executable depth walk failed for "+symbol, nil)
				return
			}

			if price.Instrument != nil && !price.Tradable(symbol, qNorm, book.BestAsk().Price) {
				err = errnie.Err(errnie.Validation, "price: allocated quantity below instrument minimum for "+symbol, nil)
				return
			}

			entryPrice := gross.Div(filled)
			entryFee := total.Sub(gross)
			exitFactor := price.WithFee(symbol, decimalOne, SELL)

			if exitFactor == nil {
				err = errnie.Err(errnie.NotFound, "price: exit factor unavailable for "+symbol, nil)
				return
			}

			breakEvenGross := total.Div(exitFactor)
			exitFee := breakEvenGross.Sub(total)
			midpoint := book.Midpoint()

			cost = &EntryCost{
				Total:              total,
				Quantity:           qNorm,
				EntryPrice:         entryPrice,
				BestAsk:            book.BestAsk().Price,
				BestBid:            book.BestBid().Price,
				Midpoint:           midpoint,
				GrossNotional:      gross,
				EntryFee:           entryFee,
				ExitFeeAtBreakEven: exitFee,
				RoundTripFees:      entryFee.Add(exitFee),
				BreakEven:          breakEvenGross.Div(filled),
				Spread:             book.BestAsk().Price.Sub(midpoint),
				Impact:             entryPrice.Sub(book.BestAsk().Price),
			}
		})
	}

	if cost != nil {
		return cost, nil
	}

	if err != nil && !errnie.IsNotFound(err) {
		return nil, reported(err)
	}

	tick := price.Tick(symbol)
	var askPrice *decimal.Decimal

	if tick != nil && tick.Ask != nil && tick.Ask.Sign() > 0 {
		askPrice = tick.Ask
	}

	if askPrice == nil {
		return nil, errnie.Error(errnie.Err(errnie.NotFound, "price: no book or quote available for "+symbol, nil))
	}

	qRaw := maxGross.Div(askPrice)
	qNorm, normErr := price.normalizer.FormatSize(symbol, qRaw)

	if normErr != nil {
		qNorm = qRaw
	}

	gross := notional(askPrice, qNorm)
	total := price.WithFee(symbol, gross, BUY)

	for total != nil && total.Cmp(budget) > 0 {
		step := qNorm.GetSmallestIncrement()

		if step == nil || step.Sign() <= 0 {
			step = decimal.NewFromFloat64(0.0001)
		}

		if qNorm.Cmp(step) <= 0 {
			return nil, errnie.Error(errnie.Err(errnie.Validation, "price: budget insufficient for minimum quote order", nil))
		}

		qNorm = qNorm.Sub(step)
		gross = notional(askPrice, qNorm)
		total = price.WithFee(symbol, gross, BUY)
	}

	if total == nil || gross == nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "price: fee calculation failed for "+symbol, nil))
	}

	entryFee := total.Sub(gross)
	exitFactor := price.WithFee(symbol, decimalOne, SELL)

	if exitFactor == nil {
		return nil, errnie.Error(errnie.Err(errnie.NotFound, "price: exit factor unavailable for "+symbol, nil))
	}

	breakEvenGross := total.Div(exitFactor)
	exitFee := breakEvenGross.Sub(total)

	cost = &EntryCost{
		Total:              total,
		Quantity:           qNorm,
		EntryPrice:         askPrice,
		BestAsk:            askPrice,
		GrossNotional:      gross,
		EntryFee:           entryFee,
		ExitFeeAtBreakEven: exitFee,
		RoundTripFees:      entryFee.Add(exitFee),
		BreakEven:          breakEvenGross.Div(qNorm),
	}

	return cost, nil
}

/*
Liquidate prices a full liquidation of the exact quantity against bid depth.
It returns net proceeds (after exit fee), gross proceeds, and any execution error.
*/
func (price *Price) Liquidate(
	symbol string,
	quantity *decimal.Decimal,
	fallbackBid ...*decimal.Decimal,
) (*decimal.Decimal, *decimal.Decimal, error) {
	if price == nil {
		return nil, nil, errnie.Error(errnie.Err(errnie.Validation, "price: price system required", nil))
	}

	if quantity == nil || quantity.Sign() <= 0 {
		return nil, nil, errnie.Error(errnie.Err(errnie.Validation, "price: positive quantity required to liquidate", nil))
	}

	var net *decimal.Decimal
	var gross *decimal.Decimal
	var err error

	if price.Books != nil {
		price.Books.Book(symbol, func(book *spotbook.Book) {
			if book == nil || book.BestBid() == nil || book.BestAsk() == nil {
				err = errnie.Err(errnie.NotFound, "price: book unavailable for "+symbol, nil)
				return
			}

			if book.BestBid().Price.Cmp(book.BestAsk().Price) >= 0 {
				price.recordAnomaly(symbol, AnomalyCrossedBook)
				err = errnie.Err(errnie.UnprocessableContent, "price: crossed book for "+symbol, nil)
				return
			}

			price.recordClean(symbol)

			filled, g, walkErr := price.Walk(book, quantity, SELL)

			if walkErr != nil {
				if errnie.IsUnprocessableContent(walkErr) {
					price.recordAnomaly(symbol, AnomalyInsufficientDepth)
				}

				err = walkErr
				return
			}

			if filled == nil || filled.Sign() <= 0 {
				err = errnie.Err(errnie.Validation, "price: zero fill during liquidation for "+symbol, nil)
				return
			}

			gross = g
			net = price.WithFee(symbol, gross, SELL)
		})
	}

	if net != nil && gross != nil {
		return net, gross, nil
	}

	if err != nil && !errnie.IsNotFound(err) {
		return nil, nil, reported(err)
	}

	var bidPrice *decimal.Decimal

	if len(fallbackBid) > 0 && fallbackBid[0] != nil && fallbackBid[0].Sign() > 0 {
		bidPrice = fallbackBid[0]
	}

	if bidPrice == nil {
		if tick := price.Tick(symbol); tick != nil && tick.Bid != nil && tick.Bid.Sign() > 0 {
			bidPrice = tick.Bid
		}
	}

	if bidPrice == nil {
		return nil, nil, errnie.Error(errnie.Err(errnie.NotFound, "price: no book or bid price available for "+symbol, nil))
	}

	gross = notional(bidPrice, quantity)
	net = price.WithFee(symbol, gross, SELL)

	if net == nil {
		return nil, nil, errnie.Error(errnie.Err(errnie.NotFound, "price: exit fee unavailable for "+symbol, nil))
	}

	return net, gross, nil
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
func (price *Price) SellQuote(
	symbol string, quantity *decimal.Decimal,
) (*types.ExecutionSurface, error) {
	return price.Surface(symbol, quantity, time.Now().UTC())
}

/* Surface prices a complete liquidation; incomplete depth stays explicit. */
func (price *Price) Surface(
	symbol string, quantity *decimal.Decimal, at time.Time,
) (*types.ExecutionSurface, error) {
	surface := &types.ExecutionSurface{Symbol: symbol, At: at, SellableQty: quantity}
	var err error

	if price.Books == nil {
		return nil, errnie.Error(
			errnie.Err(
				errnie.NotFound,
				"[price] books unavailable",
				nil,
			),
		)
	}

	price.Books.Book(symbol, func(book *spotbook.Book) {
		if book == nil || book.BestBid() == nil || book.BestAsk() == nil {
			err = errnie.Err(
				errnie.NotFound,
				"[price] book unavailable for "+symbol,
				nil,
			)

			return
		}

		// A crossed or touching book prices no liquidation; it reads as
		// book shape, not as a fault. See EntryCost above.
		if book.BestBid().Price.Cmp(book.BestAsk().Price) >= 0 {
			price.recordAnomaly(symbol, AnomalyCrossedBook)

			err = errnie.Err(
				errnie.UnprocessableContent,
				"[price] crossed book for "+symbol,
				nil,
			)

			return
		}

		price.recordClean(symbol)

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

/* GetFees normalizes fee keys once and requires a complete batch for symbols. */
func (price *Price) GetFees(symbols []string) error {
	result, err := price.loadFees(symbols)

	if err != nil {
		return err
	}

	if result == nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"price: trade volume result is required",
			nil,
		))
	}

	for symbol, fee := range result.Fees {
		price.SetFee(symbol, fee)
	}

	missing := make([]string, 0)

	for _, symbol := range symbols {
		if price.FeeIfAvailable(symbol) == nil {
			missing = append(missing, symbol)
		}
	}

	if len(missing) > 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"price: fees missing for "+strings.Join(missing, ","),
			nil,
		))
	}

	if price.Status() == runtime.WAITING {
		price.Transition(runtime.READY)
	}

	return nil
}

func (price *Price) loadFees(symbols []string) (*kraken.TradeVolumeResult, error) {
	if paper, ok := price.private.(*Paper); ok {
		return paper.TradeVolume(symbols)
	}

	client := spot.NewREST()
	client.PublicKey = os.Getenv("KRAKEN_API_KEY")
	client.PrivateKey = os.Getenv("KRAKEN_API_SECRET")

	if nonce, err := kraken.ProcessAuthNonce(); err == nil && nonce != nil {
		client.Nonce = nonce.Next
	}

	req, err := client.NewRequest(spot.RequestOptions{
		Method: "POST",
		Path:   system.Cfg.WebSocket.Endpoints.TradeVolume,
		Body:   kraken.NewTradeVolumeRequest(symbols),
	})

	if err != nil {
		return nil, err
	}

	resp, err := req.Do()

	if err != nil {
		return nil, err
	}

	result := kraken.NewTradeVolume(resp.Body)

	if result == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to decode trade volume",
			nil,
		))
	}

	return result, nil
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

func (price *Price) recordClean(symbol string) {
	if price == nil || price.anomalies == nil {
		return
	}

	price.anomalies.RecordClean(price.normalize(symbol))
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
		return 0.0
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
