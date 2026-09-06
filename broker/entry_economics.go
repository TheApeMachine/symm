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
	grossNotional := Notional(ask, quantity)
	depthGross, depthAsk, depthBid, depthKnown := price.entryDepthCost(symbol, quantity)

	if depthKnown && (depthAsk == nil || depthBid == nil) {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "entry cost: resident book requires both executable sides", nil))
	}

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

		if depthGross == nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"entry cost: visible ask depth cannot execute complete quantity",
				nil,
			))
		}

		ask = depthAsk
		bid = depthBid
		grossNotional = depthGross
	} else {
		if tick.AskQty <= 0 || quantity.Cmp(decimal.NewFromFloat64(tick.AskQty)) > 0 {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"entry cost: visible ask quantity cannot execute complete entry",
				nil,
			))
		}
	}

	var pricing Pricing

	if err := pricing.SetFee(fee.Fee); err != nil {
		return nil, err
	}
	return pricing.EntryCost(grossNotional, ask, bid, quantity), nil
}

/*
entryDepthCost walks the resident book's ask chain best-first and returns the
exact swept notional, best ask, and best bid for a complete long fill. It
preserves best quotes when depth is insufficient or crossed, so callers
reject that book instead of silently substituting ticker liquidity.
*/
func (price *Price) entryDepthCost(
	symbol string,
	quantity *decimal.Decimal,
) (*decimal.Decimal, *decimal.Decimal, *decimal.Decimal, bool) {
	if price == nil || price.api == nil || symbol == "" || quantity == nil || quantity.Sign() <= 0 {
		return nil, nil, nil, false
	}

	var entryGross, bestAsk, bestBid *decimal.Decimal
	depthKnown := false
	price.api.Book(price.api.Normalizer().Name(symbol), func(managed *spotbook.Book) {
		depthKnown = managed != nil
		if managed == nil || managed.Asks == nil || managed.Asks.Low == nil ||
			managed.Bids == nil || managed.Bids.High == nil {
			return
		}

		if managed.Asks.Low.Price == nil || managed.Bids.High.Price == nil {
			return
		}

		bestAsk = managed.Asks.Low.Price.Copy()
		bestBid = managed.Bids.High.Price.Copy()

		var pricing Pricing
		filled, gross := pricing.Sweep(managed, quantity.Rat(), nil, true, nil, nil)

		if filled.Cmp(quantity.Rat()) != 0 {
			return
		}
		entryGross = PriceDecimal(gross)
	})

	return entryGross, bestAsk, bestBid, depthKnown
}
