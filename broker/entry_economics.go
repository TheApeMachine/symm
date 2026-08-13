package broker

import (
	"fmt"
	"math"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
EntryEconomics states one candidate's forecast and current round-trip costs in
midpoint-return units. Impact is the additional round-trip cost of walking the
authoritative Level 3 book beyond its best quotes.
*/
type EntryEconomics struct {
	ExpectedReturn *decimal.Decimal
	ExpectedFees   *decimal.Decimal
	ExpectedSpread *decimal.Decimal
	ExpectedImpact *decimal.Decimal
	NetReturn      *decimal.Decimal
}

/*
ProfitableQuantity returns the capital-limited quantity whose marginal book
segments still pay entry, forecast move, exit, and both taker fees. Since asks
worsen upward and bids worsen downward, stopping at the first non-positive
segment maximizes expected absolute P&L without a guessed liquidity haircut.
*/
func (price *Price) ProfitableQuantity(
	symbol string,
	requested *decimal.Decimal,
	forecastLogReturn float64,
) (*decimal.Decimal, error) {
	if symbol == "" || requested == nil || requested.Sign() <= 0 {
		return nil, fmt.Errorf("entry quantity: symbol and positive request required")
	}

	tick := price.Tick(symbol)
	fee := price.Fee(symbol)

	if tick == nil || tick.Ask == nil || tick.Bid == nil ||
		tick.Ask.Sign() <= 0 || tick.Bid.Sign() <= 0 ||
		fee == nil || fee.Fee == nil || fee.Fee.Sign() < 0 ||
		fee.Fee.Cmp(decimal.NewFromInt64(100)) >= 0 {
		return nil, fmt.Errorf(
			"entry quantity: executable quotes and valid taker fee required",
		)
	}

	if tick.Ask.Cmp(tick.Bid) < 0 {
		return nil, fmt.Errorf(
			"entry quantity: crossed best quotes cannot price a long entry",
		)
	}

	feeRate := decimal.NewFromInt64(0).Add(fee.Fee).Div(decimal.NewFromInt64(100))
	expectedArithmeticReturn := decimal.NewFromFloat64(math.Expm1(forecastLogReturn))
	one := decimal.NewFromInt64(1)
	entryFactor := one.Add(feeRate)
	exitFactor := one.Sub(feeRate)
	executable := decimal.NewFromInt64(0)
	bookObserved := false
	bookCrossed := false
	price.api.Book(price.api.Normalizer().Name(symbol), func(managed *book.Book) {
		if managed == nil || managed.Asks.Low == nil || managed.Bids.High == nil {
			return
		}

		bookObserved = true

		if managed.Asks.Low.Price.Cmp(managed.Bids.High.Price) < 0 {
			bookCrossed = true
			return
		}

		midpoint := decimal.NewFromInt64(0).Add(managed.Asks.Low.Price).Add(
			managed.Bids.High.Price,
		).Div(
			decimal.NewFromInt64(2),
		)
		expectedMove := midpoint.Mul(expectedArithmeticReturn)
		remaining := decimal.NewFromInt64(0).Add(requested)
		askLevel := managed.Asks.Low
		bidLevel := managed.Bids.High
		askRemaining := decimal.NewFromInt64(0).Add(askLevel.Quantity)
		bidRemaining := decimal.NewFromInt64(0).Add(bidLevel.Quantity)

		for remaining.Sign() > 0 && askLevel != nil && bidLevel != nil {
			entryValue := decimal.NewFromInt64(0).Add(askLevel.Price).Mul(entryFactor)
			exitValue := decimal.NewFromInt64(0).Add(bidLevel.Price).Add(
				expectedMove,
			).Mul(exitFactor)

			if exitValue.Cmp(entryValue) <= 0 {
				return
			}

			segment := remaining

			if askRemaining.Cmp(segment) < 0 {
				segment = askRemaining
			}

			if bidRemaining.Cmp(segment) < 0 {
				segment = bidRemaining
			}

			executable = executable.Add(segment)
			remaining = remaining.Sub(segment)
			askRemaining = askRemaining.Sub(segment)
			bidRemaining = bidRemaining.Sub(segment)

			if askRemaining.Sign() == 0 {
				askLevel = askLevel.Higher

				if askLevel != nil {
					askRemaining = decimal.NewFromInt64(0).Add(askLevel.Quantity)
				}
			}

			if bidRemaining.Sign() == 0 {
				bidLevel = bidLevel.Lower

				if bidLevel != nil {
					bidRemaining = decimal.NewFromInt64(0).Add(bidLevel.Quantity)
				}
			}
		}
	})

	if bookCrossed {
		return nil, fmt.Errorf(
			"entry quantity: crossed visible book cannot price a long entry",
		)
	}

	if bookObserved && executable.Sign() <= 0 {
		return nil, fmt.Errorf("entry quantity: no visible book segment clears execution costs")
	}

	if bookObserved {
		return executable, nil
	}

	if tick.AskQty <= 0 || tick.BidQty <= 0 {
		return nil, fmt.Errorf("entry quantity: positive visible quotes required")
	}

	midpoint := decimal.NewFromInt64(0).Add(tick.Ask).Add(tick.Bid).Div(
		decimal.NewFromInt64(2),
	)
	expectedMove := midpoint.Mul(expectedArithmeticReturn)
	entryValue := decimal.NewFromInt64(0).Add(tick.Ask).Mul(entryFactor)
	exitValue := decimal.NewFromInt64(0).Add(tick.Bid).Add(expectedMove).Mul(exitFactor)

	if exitValue.Cmp(entryValue) <= 0 {
		return nil, fmt.Errorf("entry quantity: best quotes do not clear execution costs")
	}

	executable = decimal.NewFromFloat64(min(tick.AskQty, tick.BidQty))

	if requested.Cmp(executable) < 0 {
		executable = decimal.NewFromInt64(0).Add(requested)
	}

	return executable, nil
}

/*
EntryEconomics prices the forecast move from the midpoint where resonance
learns log returns, converts it to an arithmetic return, then applies that move
to the executable bid. The resulting exit value must pay the current spread and
both taker fees before it has edge.
*/
func (price *Price) EntryEconomics(
	symbol string,
	quantity *decimal.Decimal,
	forecastLogReturn float64,
) (*EntryEconomics, error) {
	if symbol == "" || quantity == nil || quantity.Sign() <= 0 {
		return nil, fmt.Errorf("entry economics: symbol and positive quantity required")
	}

	tick := price.Tick(symbol)
	fee := price.Fee(symbol)

	if tick == nil || tick.Ask == nil || tick.Ask.Sign() <= 0 ||
		tick.Bid == nil || tick.Bid.Sign() <= 0 || fee == nil ||
		fee.Fee == nil || fee.Fee.Sign() < 0 ||
		fee.Fee.Cmp(decimal.NewFromInt64(100)) >= 0 {
		return nil, fmt.Errorf("entry economics: executable quotes and taker fee required")
	}

	if tick.Ask.Cmp(tick.Bid) < 0 {
		return nil, fmt.Errorf("entry economics: crossed best quotes cannot price a long entry")
	}

	if tick.AskQty <= 0 || tick.BidQty <= 0 {
		return nil, fmt.Errorf("entry economics: positive best-quote quantities required")
	}

	ask := decimal.NewFromInt64(0).Add(tick.Ask)
	bid := decimal.NewFromInt64(0).Add(tick.Bid)
	entryPrice := ask
	exitBasePrice := bid
	depthEntry, depthExit, depthAsk, depthBid := price.depthVWAPs(symbol, quantity)

	if depthAsk != nil && depthBid != nil {
		if depthAsk.Cmp(depthBid) < 0 {
			return nil, fmt.Errorf(
				"entry economics: crossed visible book cannot price a long entry",
			)
		}

		if depthEntry == nil || depthExit == nil {
			return nil, fmt.Errorf(
				"entry economics: visible depth cannot execute complete quantity",
			)
		}

		ask = depthAsk
		bid = depthBid
		entryPrice = depthEntry
		exitBasePrice = depthExit
	}

	if depthAsk == nil && (quantity.Cmp(decimal.NewFromFloat64(tick.AskQty)) > 0 ||
		quantity.Cmp(decimal.NewFromFloat64(tick.BidQty)) > 0) {
		return nil, fmt.Errorf(
			"entry economics: visible depth cannot execute complete quantity",
		)
	}

	midpoint := ask.Add(bid).Div(decimal.NewFromInt64(2))
	expectedReturn := decimal.NewFromFloat64(math.Expm1(forecastLogReturn))
	expectedMove := midpoint.Mul(expectedReturn)
	expectedExit := exitBasePrice.Add(expectedMove)

	if expectedExit.Sign() <= 0 {
		return nil, fmt.Errorf("entry economics: forecast implies a non-positive exit price")
	}

	feeRate := decimal.NewFromInt64(0).Add(fee.Fee).Div(decimal.NewFromInt64(100))
	entryFee := entryPrice.Mul(feeRate)
	exitFee := expectedExit.Mul(feeRate)
	totalFees := entryFee.Add(exitFee)
	entryCost := entryPrice.Add(entryFee)
	exitValue := expectedExit.Sub(exitFee)
	netValue := exitValue.Sub(entryCost)
	impact := entryPrice.Sub(ask).Add(bid.Sub(exitBasePrice)).Div(midpoint)

	return &EntryEconomics{
		ExpectedReturn: expectedReturn,
		ExpectedFees:   totalFees.Div(midpoint),
		ExpectedSpread: ask.Sub(bid).Div(midpoint),
		ExpectedImpact: impact,
		NetReturn:      netValue.Div(midpoint),
	}, nil
}

func (price *Price) depthVWAPs(
	symbol string,
	quantity *decimal.Decimal,
) (*decimal.Decimal, *decimal.Decimal, *decimal.Decimal, *decimal.Decimal) {
	var entryPrice *decimal.Decimal
	var exitPrice *decimal.Decimal
	var bestAsk *decimal.Decimal
	var bestBid *decimal.Decimal
	price.api.Book(price.api.Normalizer().Name(symbol), func(managed *book.Book) {
		if managed == nil || managed.Asks.Low == nil || managed.Bids.High == nil {
			return
		}

		bestAsk = decimal.NewFromInt64(0).Add(managed.Asks.Low.Price)
		bestBid = decimal.NewFromInt64(0).Add(managed.Bids.High.Price)
		entryPrice = price.depthVWAP(managed.Asks.Low, quantity, BUY)
		exitPrice = price.depthVWAP(managed.Bids.High, quantity, SELL)
	})

	return entryPrice, exitPrice, bestAsk, bestBid
}

func (price *Price) depthVWAP(
	level *book.Level,
	quantity *decimal.Decimal,
	direction Direction,
) *decimal.Decimal {
	remaining := decimal.NewFromInt64(0).Add(quantity)
	gross := decimal.NewFromInt64(0)

	for level != nil && remaining.Sign() > 0 {
		fillQuantity := level.Quantity

		if fillQuantity.Cmp(remaining) > 0 {
			fillQuantity = remaining
		}

		gross = gross.Add(
			decimal.NewFromInt64(0).Add(level.Price).Mul(fillQuantity),
		)
		remaining = remaining.Sub(fillQuantity)

		if direction == BUY {
			level = level.Higher
			continue
		}

		level = level.Lower
	}

	if remaining.Sign() > 0 {
		return nil
	}

	return gross.Div(quantity)
}
