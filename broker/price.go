package broker

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
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
	trades        *sync.Map
	quotes        *sync.Map
	normalizer    *spot.Normalizer
	referenceCash atomic.Pointer[decimal.Decimal]
	// Flow, when set, records every trade's signed volume for the
	// participation limit of position sizing.
	Flow *Flow
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
		trades:     &sync.Map{},
		quotes:     &sync.Map{},
	}

	if private != nil && instrument != nil {
		price.Transition(runtime.WAITING)
		return price
	}

	price.Transition(runtime.READY)
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
			"[price] normalizer required for asset pair seeding",
			nil,
		))
	}

	if err := n.Use(spot.NewREST()); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[price] failed to load asset pair normalizer from public REST",
			err,
		))
	}

	return nil
}

type Quote struct {
	Bid *decimal.Decimal
	Ask *decimal.Decimal
}

func (price *Price) Update(trade *kraken.TradeData) {
	if trade == nil {
		return
	}

	normalized := price.normalize(trade.Symbol)
	price.trades.Store(normalized, &trade.Price)
	price.Flow.Record(normalized, trade.Timestamp, trade.Side, trade.Qty)
}

func (price *Price) SetQuote(symbol string, bid, ask *decimal.Decimal) {
	if price == nil {
		return
	}

	normalized := price.normalize(symbol)
	price.quotes.Store(normalized, Quote{Bid: bid, Ask: ask})
}

func (price *Price) Touch(symbol string) (*decimal.Decimal, *decimal.Decimal) {
	if price == nil {
		return nil, nil
	}

	normalized := price.normalize(symbol)
	var bid, ask *decimal.Decimal

	if price.Books != nil {
		price.Books.Book(symbol, func(book *spotbook.Book) {
			if book == nil {
				return
			}

			if bestBid := book.BestBid(); bestBid != nil {
				bid = bestBid.Price
			}

			if bestAsk := book.BestAsk(); bestAsk != nil {
				ask = bestAsk.Price
			}
		})
	}

	if (bid == nil || ask == nil) && price.quotes != nil {
		if val, ok := price.quotes.Load(normalized); ok {
			q := val.(Quote)

			if bid == nil {
				bid = q.Bid
			}

			if ask == nil {
				ask = q.Ask
			}
		}
	}

	if (bid == nil || ask == nil) && price.trades != nil {
		if val, ok := price.trades.Load(normalized); ok {
			last := val.(*decimal.Decimal)

			if bid == nil {
				bid = last
			}

			if ask == nil {
				ask = last
			}
		}
	}

	return bid, ask
}

/* Mark returns the current unit price including the taker fee. */
func (price *Price) Mark(symbol string, side Direction) *decimal.Decimal {
	bid, ask := price.Touch(symbol)

	if side == BUY {
		if ask == nil {
			return nil
		}

		return price.WithFee(symbol, ask, side)
	}

	if bid == nil {
		return nil
	}

	return price.WithFee(symbol, bid, side)
}

/*
notional uses the SDK's working precision for products of differently scaled
price and quantity inputs. Venue order formatting is performed by Normalizer.
*/
func notional(unit, quantity *decimal.Decimal) *decimal.Decimal {
	return unit.SetScale(decimal.DefaultScale).Mul(quantity)
}

/* CurrentMark returns the best observable reference price for a symbol. */
func (price *Price) CurrentMark(symbol string) *decimal.Decimal {
	if price == nil {
		return nil
	}

	bid, ask := price.Touch(symbol)

	if bid != nil {
		return bid
	}

	return ask
}

func (price *Price) RealizedReturn(
	entryCost, entryFee, exitCost, exitFee *decimal.Decimal,
) float64 {
	if entryCost == nil || entryCost.Sign() <= 0 || exitCost == nil {
		return 0
	}

	totalEntry := entryCost
	if entryFee != nil && entryFee.Sign() > 0 {
		totalEntry = entryCost.Add(entryFee)
	}

	netExit := exitCost
	if exitFee != nil && exitFee.Sign() > 0 {
		netExit = exitCost.Sub(exitFee)
	}

	profit := netExit.Sub(totalEntry)
	return profit.Div(totalEntry).Mul(decimalHundred).Float64()
}

/* Quantity sizes a buy against visible asks and available cash. */
func (price *Price) Quantity(symbol string, cash *decimal.Decimal) (*decimal.Decimal, error) {
	if cash == nil || cash.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[price] positive cash required", nil,
		))
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
		_, ask := price.Touch(symbol)

		if ask != nil {
			unit = ask
			quantity, err = price.Affordable(symbol, cash, unit)
		}
	}

	if quantity == nil && err == nil {
		err = errnie.Error(errnie.Err(
			errnie.NotFound, "[price] book unavailable for "+symbol, nil,
		))
	}

	// Cash too small for QtyMin/CostMin → abstain, do not round up into an
	// unaffordable venue order (paper CLI: Insufficient USD Available).
	if err == nil && quantity != nil && unit != nil && price.Instrument != nil {
		if !price.Tradable(symbol, quantity, unit) {
			err = errnie.Err(
				errnie.Validation,
				"[price] available cash below minimum order for "+symbol,
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
			errnie.NotFound, "[price] entry fee unavailable for "+symbol, nil,
		))
	}

	requested := cash.SetScale(decimal.DefaultScale).Div(cost)
	quantity, err := price.normalizer.FormatSize(symbol, requested)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[price] cannot normalize quantity for "+symbol,
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
			errnie.Validation, "[price] valid book and positive quantity required", nil,
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
		return quantity, gross, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "[price] insufficient book depth", nil,
		))
	}

	return quantity, gross, nil
}

/* EntryCost prices a complete entry from the resident book and current fee. */
func (price *Price) EntryCost(symbol string, quantity *decimal.Decimal) (*EntryCost, error) {
	if quantity == nil || quantity.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "entry cost: positive quantity required", nil,
		))
	}

	var cost *EntryCost
	var err error

	if price.Books == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound, "entry cost: books unavailable", nil,
		))
	}

	price.Books.Book(symbol, func(book *spotbook.Book) {
		if book == nil || book.BestAsk() == nil || book.BestBid() == nil {
			err = errnie.Error(errnie.Err(
				errnie.NotFound, "entry cost: book unavailable for "+symbol, nil,
			))
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
			err = errnie.Error(errnie.Err(
				errnie.UnprocessableContent, "entry cost: crossed book for "+symbol, nil,
			))
			return
		}

		filled, gross, err := price.Walk(book, quantity, BUY)

		if err != nil {
			err = errnie.Error(errnie.Err(
				errnie.IO, "[price] get entry cost failed for "+symbol, err,
			))
			return
		}

		total := price.WithFee(symbol, gross, BUY)
		exitFactor := price.WithFee(symbol, decimalOne, SELL)

		if total == nil || exitFactor == nil {
			err = errnie.Error(errnie.Err(
				errnie.NotFound, "entry cost: fee unavailable for "+symbol, nil,
			))
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
		err = errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"entry cost: complete executable book required for "+symbol,
			nil,
		))
	}

	return cost, err
}

/*
budget is the reference cash an allocation may spend. There is no fixed
fraction: live entries are sized by the Desk from exit capacity, flow noise
and this cash, never by a constant share of it.
*/
func (price *Price) budget(referenceCash ...*decimal.Decimal) (*decimal.Decimal, error) {
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
			"[price] positive reference cash required to allocate entry",
			nil,
		))
	}

	return cash.SetScale(decimal.DefaultScale), nil
}

/*
RoundTrip prices one complete long at the allocation budget: the quantity the
budget affords at entry, bought at entry and sold at exit, both legs paying the
taker fee. It returns the net PnL in quote currency and the total entry cost.
*/
func (price *Price) RoundTrip(
	symbol string, entry, exit *decimal.Decimal,
) (*decimal.Decimal, *decimal.Decimal, error) {
	if entry == nil || exit == nil || entry.Sign() <= 0 || exit.Sign() <= 0 {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.Validation, "[price] positive entry and exit prices required for "+symbol, nil,
		))
	}

	if price.Fee(symbol) == nil {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.NotFound, "[price] fee unavailable for "+symbol, nil,
		))
	}

	budget, err := price.budget()

	if err != nil {
		return nil, nil, errnie.Error(err)
	}

	quantity, err := price.Affordable(symbol, budget, entry)

	if err != nil {
		return nil, nil, errnie.Error(err)
	}

	if quantity.Sign() <= 0 {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.Validation, "[price] budget affords no quantity of "+symbol, nil,
		))
	}

	total := price.WithFee(symbol, notional(entry, quantity), BUY)
	net := price.WithFee(symbol, notional(exit, quantity), SELL)

	return net.Sub(total), total, nil
}

/*
AllocateEntry prices an executable entry frozen at the allocation budget.
It walks ask depth to derive the maximum executable quantity such that
gross notional plus taker entry fee does not exceed the budget.
*/
func (price *Price) AllocateEntry(
	symbol string,
	referenceCash ...*decimal.Decimal,
) (*EntryCost, error) {
	if price == nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "[price] price system required", nil))
	}

	budget, err := price.budget(referenceCash...)

	if err != nil {
		return nil, err
	}

	fee := price.Fee(symbol)

	if fee == nil || fee.Fee == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound, "[price] fee unavailable for "+symbol, nil,
		))
	}

	feeRate := decimalPercent.Mul(fee.Fee)
	maxGross := budget.Div(decimalOne.Add(feeRate))

	var cost *EntryCost

	if price.Books != nil {
		price.Books.Book(symbol, func(book *spotbook.Book) {
			if book == nil || book.BestAsk() == nil || book.BestBid() == nil {
				err = errnie.Error(errnie.Err(
					errnie.NotFound, "[price] book unavailable for "+symbol, nil,
				))
				return
			}

			if book.BestBid().Price.Cmp(book.BestAsk().Price) >= 0 {
				err = errnie.Error(errnie.Err(
					errnie.UnprocessableContent, "[price] crossed book for "+symbol, nil,
				))
				return
			}

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
				err = errnie.Error(errnie.Err(
					errnie.Validation, "[price] budget insufficient to fill any ask depth for "+symbol, nil,
				))
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
					err = errnie.Error(errnie.Err(
						errnie.Validation,
						"[price] budget insufficient for minimum tradable quantity for "+symbol,
						nil,
					))
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
				err = errnie.Error(errnie.Err(
					errnie.Validation,
					"[price] executable depth walk failed for "+symbol,
					nil,
				))
				return
			}

			if price.Instrument != nil && !price.Tradable(symbol, qNorm, book.BestAsk().Price) {
				err = errnie.Error(errnie.Err(
					errnie.Validation,
					"[price] allocated quantity below instrument minimum for "+symbol,
					nil,
				))
				return
			}

			entryPrice := gross.Div(filled)
			entryFee := total.Sub(gross)
			exitFactor := price.WithFee(symbol, decimalOne, SELL)

			if exitFactor == nil {
				err = errnie.Error(errnie.Err(
					errnie.NotFound, "[price] exit factor unavailable for "+symbol, nil,
				))
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
		return nil, errnie.Error(errnie.Err(
			errnie.IO, "[price] get entry cost failed for "+symbol, err,
		))
	}

	_, askPrice := price.Touch(symbol)

	if askPrice == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound, "[price] no book or quote available for "+symbol, nil,
		))
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
			return nil, errnie.Error(errnie.Err(
				errnie.Validation, "[price] budget insufficient for minimum quote order", nil,
			))
		}

		qNorm = qNorm.Sub(step)
		gross = notional(askPrice, qNorm)
		total = price.WithFee(symbol, gross, BUY)
	}

	if total == nil || gross == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[price] fee calculation failed for "+symbol, nil,
		))
	}

	entryFee := total.Sub(gross)
	exitFactor := price.WithFee(symbol, decimalOne, SELL)

	if exitFactor == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound, "[price] exit factor unavailable for "+symbol, nil,
		))
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
		return nil, nil, errnie.Error(errnie.Err(
			errnie.Validation, "[price] price system required", nil,
		))
	}

	if quantity == nil || quantity.Sign() <= 0 {
		return nil, nil, errnie.Error(errnie.Err(
			errnie.Validation, "[price] positive quantity required to liquidate", nil,
		))
	}

	var net *decimal.Decimal
	var gross *decimal.Decimal
	var err error

	if price.Books != nil {
		price.Books.Book(symbol, func(book *spotbook.Book) {
			if book == nil || book.BestBid() == nil || book.BestAsk() == nil {
				err = errnie.Error(errnie.Err(
					errnie.NotFound, "[price] book unavailable for "+symbol, nil,
				))
				return
			}

			if book.BestBid().Price.Cmp(book.BestAsk().Price) >= 0 {
				err = errnie.Error(errnie.Err(
					errnie.UnprocessableContent, "[price] crossed book for "+symbol, nil,
				))
				return
			}

			filled, g, err := price.Walk(book, quantity, SELL)

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.IO, "[price] liquidate failed for "+symbol, err,
				))
				return
			}

			if filled == nil || filled.Sign() <= 0 {
				err = errnie.Error(errnie.Err(
					errnie.Validation, "[price] zero fill during liquidation for "+symbol, nil,
				))

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
		return nil, nil, errnie.Error(errnie.Err(errnie.IO, "[price] liquidate failed for "+symbol, err))
	}

	var bidPrice *decimal.Decimal

	if len(fallbackBid) > 0 && fallbackBid[0] != nil && fallbackBid[0].Sign() > 0 {
		bidPrice = fallbackBid[0]
	}

	if bidPrice == nil {
		bidPrice, _ = price.Touch(symbol)
	}

	if bidPrice == nil {
		return nil, nil, errnie.Error(errnie.Err(errnie.NotFound, "[price] no book or bid price available for "+symbol, nil))
	}

	gross = notional(bidPrice, quantity)
	net = price.WithFee(symbol, gross, SELL)

	if net == nil {
		return nil, nil, errnie.Error(errnie.Err(errnie.NotFound, "[price] exit fee unavailable for "+symbol, nil))
	}

	return net, gross, nil
}

/*
SellQuote prices a complete liquidation from visible bids and current fee.
*/
func (price *Price) SellQuote(
	symbol string, quantity *decimal.Decimal,
) (*types.ExecutionSurface, error) {
	surface := &types.ExecutionSurface{
		Symbol:      symbol,
		At:          time.Now().UTC(),
		SellableQty: quantity,
	}

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
			errnie.Error(errnie.Err(
				errnie.NotFound,
				"[price] book unavailable for "+symbol,
				nil,
			))

			return
		}

		// A crossed or touching book prices no liquidation; it reads as
		// book shape, not as a fault. See EntryCost above.
		if book.BestBid().Price.Cmp(book.BestAsk().Price) >= 0 {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"[price] crossed book for "+symbol,
				nil,
			))

			return
		}

		surface.BookComplete = true
		surface.BestBid = book.BestBid().Price
		surface.ExecutableQty = decimalZero

		for bid := book.BestBid(); bid != nil; bid = bid.Lower {
			surface.ExecutableQty = surface.ExecutableQty.Add(bid.Quantity)
		}

		filled, gross, err := price.Walk(book, quantity, SELL)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"symm: walk failed for "+symbol,
				err,
			))

			return
		}

		surface.Gross = gross
		surface.FullyExecutable = true
		surface.ExecutableVWAP = gross.Div(filled)
		surface.ExecutableValue = price.WithFee(symbol, gross, SELL)
	})

	return surface, nil
}

/*
Tradable checks the instrument's actual quantity and notional minimums.
*/
func (price *Price) Tradable(symbol string, quantity, unit *decimal.Decimal) bool {
	if price == nil || price.Instrument == nil {
		return true
	}

	pair := price.Instrument.Pair(symbol)

	if pair.QtyMin == nil || pair.CostMin == nil {
		return false
	}

	return quantity.Cmp(pair.QtyMin) >= 0 && notional(unit, quantity).Cmp(pair.CostMin) >= 0
}

/*
FeeRate is the symbol's taker fee as a fraction, and false when no fee is
known.
*/
func (price *Price) FeeRate(symbol string) (float64, bool) {
	fee := price.Fee(symbol)

	if fee == nil || fee.Fee == nil {
		return 0, false
	}

	return fee.Fee.Float64() / 100, true
}

/*
Fee returns the taker fee for a symbol.
*/
func (price *Price) Fee(symbol string) *kraken.TradeVolumeFee {
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
			"[price] trade volume result is required",
			nil,
		))
	}

	for symbol, fee := range result.Fees {
		price.SetFee(symbol, fee)
	}

	missing := make([]string, 0)

	for _, symbol := range symbols {
		if price.Fee(symbol) == nil {
			missing = append(missing, symbol)
		}
	}

	if len(missing) > 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[price] fees missing for "+strings.Join(missing, ","),
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

	client, err := kraken.NewAuthenticatedREST()

	if err != nil {
		return nil, err
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
	rate := decimalPercent.Mul(fee.Fee)

	if direction == SELL {
		rate = rate.Mul(decimalNegativeOne)
	}

	return amount.OffsetPercent(rate)
}
