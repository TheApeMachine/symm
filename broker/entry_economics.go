package broker

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
ExecutableQuantity caps one cash-sized request to the asks that are observable
now. It makes no statement about whether the market will later move far enough
to produce a profit; that is the structural thesis' job.
*/
func (price *Price) ExecutableQuantity(
	symbol string,
	requested *decimal.Decimal,
) (*decimal.Decimal, error) {
	if price == nil || symbol == "" || requested == nil || requested.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry quantity: price surface, symbol, and positive requested quantity required",
			nil,
		))
	}

	tick := price.Tick(symbol)

	if tick == nil || tick.Ask == nil || tick.Bid == nil ||
		tick.Ask.Sign() <= 0 || tick.Bid.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry quantity: executable bid and ask required",
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

	// No full-depth book is available to this signal, so the executable
	// quantity is always priced off the ticker's own visible ask quantity
	// rather than a walked ask-side depth chain.
	var visible *decimal.Decimal

	if tick.AskQty <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry quantity: positive visible ask quantity required",
			nil,
		))
	}

	visible = decimal.NewFromFloat64(tick.AskQty)

	if requested.Cmp(visible) <= 0 {
		return decimal.NewFromInt64(0).Add(requested), nil
	}

	return visible, nil
}

/*
EntryCost prices only the order that can be placed now. It reports entry VWAP,
observable spread and impact, both known fee crossings, and the fee-inclusive
break-even sale price. No future midpoint, future ask, or expected return is
constructed.
*/
func (price *Price) EntryCost(
	symbol string,
	quantity *decimal.Decimal,
) (*types.EntryCost, error) {
	if price == nil || symbol == "" || quantity == nil || quantity.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry cost: price surface, symbol, and positive quantity required",
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
			"entry cost: executable quotes and valid taker fee required",
			nil,
		))
	}

	if tick.Ask.Cmp(tick.Bid) < 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry cost: crossed best quotes cannot price a long entry",
			nil,
		))
	}

	ask := decimal.NewFromInt64(0).Add(tick.Ask)
	bid := decimal.NewFromInt64(0).Add(tick.Bid)
	entryPrice := decimal.NewFromInt64(0).Add(ask)
	depthEntry, depthAsk, depthBid := price.entryDepthVWAP(symbol, quantity)

	if depthAsk != nil {
		if depthBid == nil {
			depthBid = bid
		}

		if depthAsk.Cmp(depthBid) < 0 {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"entry cost: crossed visible book cannot price a long entry",
				nil,
			))
		}

		if depthEntry == nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"entry cost: visible ask depth cannot execute complete quantity",
				nil,
			))
		}

		ask = depthAsk
		bid = depthBid
		entryPrice = depthEntry
	} else {
		if tick.AskQty <= 0 || quantity.Cmp(decimal.NewFromFloat64(tick.AskQty)) > 0 {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"entry cost: visible ask quantity cannot execute complete entry",
				nil,
			))
		}
	}

	midpoint := decimal.NewFromInt64(0).Add(ask).Add(bid).Div(
		decimal.NewFromInt64(2),
	)
	feeRate := decimal.NewFromInt64(0).Add(fee.Fee).Div(
		decimal.NewFromInt64(100),
	)
	exitFactor := decimal.NewFromInt64(1).Sub(feeRate)

	if exitFactor.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry cost: exit fee leaves no realizable proceeds",
			nil,
		))
	}

	grossNotional := decimal.NewFromInt64(0).Add(entryPrice).Mul(quantity)
	entryFee := decimal.NewFromInt64(0).Add(grossNotional).Mul(feeRate)
	totalEntryCost := decimal.NewFromInt64(0).Add(grossNotional).Add(entryFee)
	breakEvenGross := decimal.NewFromInt64(0).Add(totalEntryCost).Div(exitFactor)
	breakEven := decimal.NewFromInt64(0).Add(breakEvenGross).Div(quantity)
	exitFeeAtBreakEven := decimal.NewFromInt64(0).Add(breakEvenGross).Mul(feeRate)
	roundTripFees := decimal.NewFromInt64(0).Add(entryFee).Add(exitFeeAtBreakEven)
	spread := decimal.NewFromInt64(0).Add(ask).Sub(midpoint)
	impact := decimal.NewFromInt64(0).Add(entryPrice).Sub(ask)

	if impact.Sign() < 0 {
		impact = decimal.NewFromInt64(0)
	}

	return &types.EntryCost{
		EntryPrice:         decimal.NewFromInt64(0).Add(entryPrice),
		BestAsk:            decimal.NewFromInt64(0).Add(ask),
		BestBid:            decimal.NewFromInt64(0).Add(bid),
		Midpoint:           decimal.NewFromInt64(0).Add(midpoint),
		GrossNotional:      grossNotional,
		EntryFee:           entryFee,
		ExitFeeAtBreakEven: exitFeeAtBreakEven,
		RoundTripFees:      roundTripFees,
		Spread:             spread,
		Impact:             impact,
		BreakEven:          breakEven,
	}, nil
}

/*
entryDepthVWAP always reports unavailable: this signal has no access to a
full-depth book to walk for a multi-level VWAP, so EntryCost's caller always
takes its ticker-level fallback path instead.
*/
func (price *Price) entryDepthVWAP(
	symbol string,
	quantity *decimal.Decimal,
) (*decimal.Decimal, *decimal.Decimal, *decimal.Decimal) {
	return nil, nil, nil
}
