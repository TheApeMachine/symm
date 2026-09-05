package broker

import (
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
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
		return requested.Copy(), nil
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
		fee.Fee.Cmp(decimalHundred) >= 0 {
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

	ask := tick.Ask.Copy()
	bid := tick.Bid.Copy()
	entryPrice := ask.Copy()
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

	midpoint := decimalZero.Add(ask).Add(bid).Div(decimalTwo)
	feeRate := decimalZero.Add(fee.Fee).Div(decimalHundred)
	exitFactor := decimalOne.Sub(feeRate)

	if exitFactor.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry cost: exit fee leaves no realizable proceeds",
			nil,
		))
	}

	grossNotional := decimalZero.Add(entryPrice).Mul(quantity)
	entryFee := decimalZero.Add(grossNotional).Mul(feeRate)
	totalEntryCost := decimalZero.Add(grossNotional).Add(entryFee)
	breakEvenGross := decimalZero.Add(totalEntryCost).Div(exitFactor)
	breakEven := decimalZero.Add(breakEvenGross).Div(quantity)
	exitFeeAtBreakEven := decimalZero.Add(breakEvenGross).Mul(feeRate)
	roundTripFees := decimalZero.Add(entryFee).Add(exitFeeAtBreakEven)
	spread := decimalZero.Add(ask).Sub(midpoint)
	impact := decimalZero.Add(entryPrice).Sub(ask)

	if impact.Sign() < 0 {
		impact = decimalZero.Copy()
	}

	return &types.EntryCost{
		EntryPrice:         decimalZero.Add(entryPrice),
		BestAsk:            decimalZero.Add(ask),
		BestBid:            decimalZero.Add(bid),
		Midpoint:           decimalZero.Add(midpoint),
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
entryDepthVWAP walks the resident book's ask chain best-first and returns the
multi-level entry VWAP, best ask, and best bid for a complete long fill. It
returns nil for every result when the resident book is not seeded or deep
enough, so callers take their documented ticker-level fallback.
*/
func (price *Price) entryDepthVWAP(
	symbol string,
	quantity *decimal.Decimal,
) (*decimal.Decimal, *decimal.Decimal, *decimal.Decimal) {
	if price == nil || price.api == nil || symbol == "" || quantity == nil || quantity.Sign() <= 0 {
		return nil, nil, nil
	}

	var entryVWAP, bestAsk, bestBid *decimal.Decimal
	price.api.Book(price.api.Normalizer().Name(symbol), func(managed *spotbook.Book) {
		if managed == nil || managed.Asks == nil || managed.Asks.Low == nil ||
			managed.Bids == nil || managed.Bids.High == nil {
			return
		}

		if managed.Asks.Low.Price == nil || managed.Bids.High.Price == nil ||
			managed.Asks.Low.Price.Cmp(managed.Bids.High.Price) < 0 {
			return
		}

		bestAsk = managed.Asks.Low.Price.Copy()
		bestBid = managed.Bids.High.Price.Copy()

		remaining := decimalZero.Add(quantity)
		grossNotional := decimalZero.Copy()

		for ask := managed.Asks.Low; ask != nil && remaining.Sign() > 0; ask = ask.Higher {
			if ask.Price == nil || ask.Quantity == nil ||
				ask.Price.Sign() <= 0 || ask.Quantity.Sign() <= 0 {
				continue
			}

			fill := ask.Quantity
			if remaining.Cmp(fill) < 0 {
				fill = remaining
			}

			grossNotional = grossNotional.Add(decimalZero.Add(ask.Price).Mul(fill))
			remaining = remaining.Sub(fill)
		}

		if remaining.Sign() > 0 {
			bestAsk, bestBid = nil, nil
			return
		}

		entryVWAP = grossNotional.Div(quantity)
	})

	return entryVWAP, bestAsk, bestBid
}
