package broker

import (
	"github.com/krakenfx/api-go/v2/pkg/book"
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

	visible := decimal.NewFromInt64(0)
	bookObserved := false
	bookCrossed := false

	if price.api != nil {
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

			for level := managed.Asks.Low; level != nil; level = level.Higher {
				if level.Quantity == nil || level.Quantity.Sign() <= 0 {
					continue
				}

				visible = visible.Add(level.Quantity)
			}
		})
	}

	if bookCrossed {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"entry quantity: crossed visible book cannot price a long entry",
			nil,
		))
	}

	if bookObserved {
		if visible.Sign() <= 0 {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"entry quantity: visible ask depth is empty",
				nil,
			))
		}

		if requested.Cmp(visible) <= 0 {
			return decimal.NewFromInt64(0).Add(requested), nil
		}

		return visible, nil
	}

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

func (price *Price) entryDepthVWAP(
	symbol string,
	quantity *decimal.Decimal,
) (*decimal.Decimal, *decimal.Decimal, *decimal.Decimal) {
	if price == nil || price.api == nil {
		return nil, nil, nil
	}

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
