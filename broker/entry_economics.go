package broker

import (
	"fmt"
	"math"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
EntryEconomics states one candidate's forecast and observable entry costs in
midpoint-return units. The exit fee is known from the venue schedule, but no
current bid depth is presented as the book that will exist at forecast expiry.
*/
type EntryEconomics struct {
	ExpectedReturn *decimal.Decimal
	ExpectedFees   *decimal.Decimal
	ExpectedSpread *decimal.Decimal
	ExpectedImpact *decimal.Decimal
	NetReturn      *decimal.Decimal
}

/*
ProfitableQuantity returns the capital-limited quantity whose observable ask
segments still clear the forecast-horizon midpoint and both taker fees. Current
bid depth describes an immediate liquidation, not the future exit book, so it
cannot cap or veto a forecast-horizon entry.
*/
func (price *Price) ProfitableQuantity(
	symbol string,
	requested *decimal.Decimal,
	forecastLogReturn float64,
	minimumNetReturn float64,
) (*decimal.Decimal, error) {
	if symbol == "" || requested == nil || requested.Sign() <= 0 ||
		minimumNetReturn < -1 || minimumNetReturn > 1 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry quantity: symbol, positive requested quantity, and minimum return within [-1,1] required",
			nil,
		))
	}

	tick := price.Tick(symbol)
	fee := price.Fee(symbol)

	if tick == nil || tick.Ask == nil || tick.Bid == nil ||
		tick.Ask.Sign() <= 0 || tick.Bid.Sign() <= 0 ||
		fee == nil || fee.Fee == nil || fee.Fee.Sign() < 0 ||
		fee.Fee.Cmp(decimal.NewFromInt64(100)) >= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry quantity: executable quotes and valid taker fee required",
			nil,
		))
	}

	if tick.Ask.Cmp(tick.Bid) < 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry quantity: crossed best quotes cannot price a long entry",
			nil,
		))
	}

	feeRate := decimal.NewFromInt64(0).Add(fee.Fee).Div(decimal.NewFromInt64(100))
	expectedArithmeticReturn := decimal.NewFromFloat64(math.Expm1(forecastLogReturn))
	one := decimal.NewFromInt64(1)
	entryFactor := one.Add(feeRate)
	exitFactor := one.Sub(feeRate)
	minimumReturn := decimal.NewFromFloat64(minimumNetReturn)
	executable := decimal.NewFromInt64(0)
	bookObserved := false
	bookCrossed := false

	price.api.Book(price.api.Normalizer().Name(symbol), func(managed *book.Book) {
		if managed == nil || managed.Asks.Low == nil {
			return
		}

		bookObserved = true
		bestBid := tick.Bid

		if managed.Bids.High != nil {
			bestBid = managed.Bids.High.Price
		}

		if managed.Asks.Low.Price.Cmp(bestBid) < 0 {
			bookCrossed = true
			return
		}

		midpoint := decimal.NewFromInt64(0).Add(managed.Asks.Low.Price).Add(
			bestBid,
		).Div(
			decimal.NewFromInt64(2),
		)
		expectedExit := midpoint.Mul(one.Add(expectedArithmeticReturn)).Mul(exitFactor)
		remaining := decimal.NewFromInt64(0).Add(requested)
		askLevel := managed.Asks.Low
		askRemaining := decimal.NewFromInt64(0).Add(askLevel.Quantity)

		for remaining.Sign() > 0 && askLevel != nil {
			entryValue := decimal.NewFromInt64(0).Add(askLevel.Price).Mul(entryFactor)
			segmentReturn := expectedExit.Sub(entryValue).Div(midpoint)

			if segmentReturn.Cmp(minimumReturn) <= 0 {
				return
			}

			segment := remaining

			if askRemaining.Cmp(segment) < 0 {
				segment = askRemaining
			}

			executable = executable.Add(segment)
			remaining = remaining.Sub(segment)
			askRemaining = askRemaining.Sub(segment)

			if askRemaining.Sign() == 0 {
				askLevel = askLevel.Higher

				if askLevel != nil {
					askRemaining = decimal.NewFromInt64(0).Add(askLevel.Quantity)
				}
			}

		}
	})

	if bookCrossed {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry quantity: crossed visible book cannot price a long entry",
			nil,
		))
	}

	if bookObserved && executable.Sign() <= 0 {
		return nil, errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"entry quantity: no visible ask segment clears forecast-horizon value for %s forecast %f log return (%.6f arithmetic return) executable quantity %s",
				symbol,
				forecastLogReturn,
				expectedArithmeticReturn.Float64(),
				executable,
			),
			nil,
		)
	}

	if bookObserved {
		return executable, nil
	}

	if tick.AskQty <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry quantity: positive visible ask quantity required",
			nil,
		))
	}

	midpoint := decimal.NewFromInt64(0).Add(tick.Ask).Add(tick.Bid).Div(
		decimal.NewFromInt64(2),
	)
	expectedExit := midpoint.Mul(one.Add(expectedArithmeticReturn)).Mul(exitFactor)
	entryValue := decimal.NewFromInt64(0).Add(tick.Ask).Mul(entryFactor)
	netReturn := expectedExit.Sub(entryValue).Div(midpoint)

	if netReturn.Cmp(minimumReturn) <= 0 {
		return nil, errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"entry quantity: observable entry costs do not clear forecast-horizon value for %s forecast %f log return (%.6f arithmetic return) expected exit value %s entry value %s executable quantity %s",
				symbol,
				forecastLogReturn,
				expectedArithmeticReturn.Float64(),
				expectedExit.String(),
				entryValue.String(),
				executable.String(),
			),
			nil,
		)
	}

	executable = decimal.NewFromFloat64(tick.AskQty)

	if requested.Cmp(executable) < 0 {
		executable = decimal.NewFromInt64(0).Add(requested)
	}

	return executable, nil
}

/*
EntryEconomics prices the forecast move from the current midpoint where
resonance learns log returns. It walks only the observable ask depth needed to
enter. The future exit is the forecast midpoint net of the known taker fee;
future spread and depth require an outcome-calibrated model and are not inferred
from the current bid book.
*/
func (price *Price) EntryEconomics(
	symbol string,
	quantity *decimal.Decimal,
	forecastLogReturn float64,
) (*EntryEconomics, error) {
	if symbol == "" || quantity == nil || quantity.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry economics: symbol and positive quantity required",
			nil,
		))
	}

	tick := price.Tick(symbol)
	fee := price.Fee(symbol)

	if tick == nil || tick.Ask == nil || tick.Ask.Sign() <= 0 ||
		tick.Bid == nil || tick.Bid.Sign() <= 0 || fee == nil ||
		fee.Fee == nil || fee.Fee.Sign() < 0 ||
		fee.Fee.Cmp(decimal.NewFromInt64(100)) >= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry economics: executable quotes and taker fee required",
			nil,
		))
	}

	if tick.Ask.Cmp(tick.Bid) < 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry economics: crossed best quotes cannot price a long entry",
			nil,
		))
	}

	if tick.AskQty <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry economics: positive visible ask quantity required",
			nil,
		))
	}

	ask := decimal.NewFromInt64(0).Add(tick.Ask)
	bid := decimal.NewFromInt64(0).Add(tick.Bid)
	entryPrice := ask
	depthEntry, depthAsk, depthBid := price.entryDepthVWAP(symbol, quantity)

	if depthAsk != nil {
		if depthBid == nil {
			depthBid = bid
		}

		if depthAsk.Cmp(depthBid) < 0 {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"entry economics: crossed visible book cannot price a long entry",
				nil,
			))
		}

		if depthEntry == nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"entry economics: visible ask depth cannot execute complete quantity",
				nil,
			))
		}

		ask = depthAsk
		bid = depthBid
		entryPrice = depthEntry
	}

	if depthAsk == nil && quantity.Cmp(decimal.NewFromFloat64(tick.AskQty)) > 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry economics: visible ask quantity cannot execute complete entry",
			nil,
		))
	}

	midpoint := ask.Add(bid).Div(decimal.NewFromInt64(2))
	expectedReturn := decimal.NewFromFloat64(math.Expm1(forecastLogReturn))
	expectedExit := midpoint.Mul(decimal.NewFromInt64(1).Add(expectedReturn))

	if expectedExit.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry economics: forecast implies a non-positive exit price",
			nil,
		))
	}

	feeRate := decimal.NewFromInt64(0).Add(fee.Fee).Div(decimal.NewFromInt64(100))
	entryFee := entryPrice.Mul(feeRate)
	exitFee := expectedExit.Mul(feeRate)
	totalFees := entryFee.Add(exitFee)
	entryCost := entryPrice.Add(entryFee)
	exitValue := expectedExit.Sub(exitFee)
	netValue := exitValue.Sub(entryCost)
	spread := ask.Sub(midpoint).Div(midpoint)
	impact := entryPrice.Sub(ask).Div(midpoint)

	return &EntryEconomics{
		ExpectedReturn: expectedReturn,
		ExpectedFees:   totalFees.Div(midpoint),
		ExpectedSpread: spread,
		ExpectedImpact: impact,
		NetReturn:      netValue.Div(midpoint),
	}, nil
}

func (price *Price) entryDepthVWAP(
	symbol string,
	quantity *decimal.Decimal,
) (*decimal.Decimal, *decimal.Decimal, *decimal.Decimal) {
	var entryPrice *decimal.Decimal
	var bestAsk *decimal.Decimal
	var bestBid *decimal.Decimal
	price.api.Book(price.api.Normalizer().Name(symbol), func(managed *book.Book) {
		if managed == nil || managed.Asks.Low == nil {
			return
		}

		bestAsk = decimal.NewFromInt64(0).Add(managed.Asks.Low.Price)

		if managed.Bids.High != nil {
			bestBid = decimal.NewFromInt64(0).Add(managed.Bids.High.Price)
		}

		entryPrice = price.askVWAP(managed.Asks.Low, quantity)
	})

	return entryPrice, bestAsk, bestBid
}

func (price *Price) askVWAP(
	level *book.Level,
	quantity *decimal.Decimal,
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

		level = level.Higher
	}

	if remaining.Sign() > 0 {
		return nil
	}

	return gross.Div(quantity)
}
