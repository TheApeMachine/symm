package broker

import (
	"fmt"
	"math"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
EntryEconomics states one candidate's forecast and current round-trip costs in
midpoint-return units. Impact is zero only when the complete quantity fits at
both current best quotes; larger candidates are refused because their depth
impact has not been priced.
*/
type EntryEconomics struct {
	ExpectedReturn *decimal.Decimal
	ExpectedFees   *decimal.Decimal
	ExpectedSpread *decimal.Decimal
	ExpectedImpact *decimal.Decimal
	NetReturn      *decimal.Decimal
}

/*
EntryEconomics prices the forecast move from the midpoint where resonance
learns returns, then applies that move to the executable bid. The resulting
exit value must pay the current spread and both taker fees before it has edge.
*/
func (price *Price) EntryEconomics(
	symbol string,
	quantity *decimal.Decimal,
	forecastReturn float64,
) (*EntryEconomics, error) {
	if symbol == "" || quantity == nil || quantity.Sign() <= 0 {
		return nil, fmt.Errorf("entry economics: symbol and positive quantity required")
	}

	if math.IsNaN(forecastReturn) || math.IsInf(forecastReturn, 0) {
		return nil, fmt.Errorf("entry economics: finite forecast return required")
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

	if math.IsNaN(tick.AskQty) || math.IsInf(tick.AskQty, 0) || tick.AskQty <= 0 ||
		math.IsNaN(tick.BidQty) || math.IsInf(tick.BidQty, 0) || tick.BidQty <= 0 {
		return nil, fmt.Errorf("entry economics: positive best-quote quantities required")
	}

	if quantity.Cmp(decimal.NewFromFloat64(tick.AskQty)) > 0 ||
		quantity.Cmp(decimal.NewFromFloat64(tick.BidQty)) > 0 {
		return nil, fmt.Errorf(
			"entry economics: quantity exceeds current best-quote liquidity; depth impact required",
		)
	}

	ask := decimal.NewFromInt64(0).Add(tick.Ask)
	bid := decimal.NewFromInt64(0).Add(tick.Bid)
	midpoint := ask.Add(bid).Div(decimal.NewFromInt64(2))
	expectedReturn := decimal.NewFromFloat64(forecastReturn)
	expectedMove := midpoint.Mul(expectedReturn)
	expectedExit := bid.Add(expectedMove)

	if expectedExit.Sign() <= 0 {
		return nil, fmt.Errorf("entry economics: forecast implies a non-positive exit price")
	}

	feeRate := decimal.NewFromInt64(0).Add(fee.Fee).Div(decimal.NewFromInt64(100))
	entryFee := ask.Mul(feeRate)
	exitFee := expectedExit.Mul(feeRate)
	totalFees := entryFee.Add(exitFee)
	entryCost := ask.Add(entryFee)
	exitValue := expectedExit.Sub(exitFee)
	netValue := exitValue.Sub(entryCost)

	return &EntryEconomics{
		ExpectedReturn: expectedReturn,
		ExpectedFees:   totalFees.Div(midpoint),
		ExpectedSpread: ask.Sub(bid).Div(midpoint),
		ExpectedImpact: decimal.NewFromInt64(0),
		NetReturn:      netValue.Div(midpoint),
	}, nil
}
