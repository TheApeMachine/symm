package response

import (
	"fmt"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

func (orders *Orders) pairMeta(symbol string) pairMeta {
	if orders.catalog != nil {
		return orders.catalog.Meta(symbol)
	}

	quote, _ := quoteAsset(symbol)

	return pairMeta{
		takerPct: defaultTakerFeePct,
		makerPct: defaultMakerFeePct,
		tickSize: defaultTickSize,
		quote:    quote,
	}
}

func (orders *Orders) feeAmount(symbol string, qty, price float64, maker bool) float64 {
	meta := orders.pairMeta(symbol)
	feePct := meta.takerPct

	if maker {
		feePct = meta.makerPct
	}

	return qty * price * feePct / 100
}

func (orders *Orders) tickSize(symbol string) float64 {
	return orders.pairMeta(symbol).tickSize
}

/*
takerFillPrice walks the cached L2 book for market-style exits and entries.
Protective take-profit market orders do not assume upside gap-through.
*/
type takerFillQuote struct {
	price         float64
	filledQty     float64
	depthCoverage float64
}

func (orders *Orders) takerFillQuote(
	symbol string,
	side trading.Side,
	qty float64,
	capPrice float64,
	action reasoning.ActionType,
) (takerFillQuote, error) {
	quote, ok := orders.quotes.Snapshot(symbol)

	if !ok {
		return takerFillQuote{}, fmt.Errorf("paper fill: no quote for %s", symbol)
	}

	var fill broker.FillQuote
	var err error

	if orders.stress != nil {
		fill, err = broker.StressedSlippageFill(quote, side, qty, orders.stress.Snapshot(symbol))
	} else {
		fill, err = broker.SlippageFill(quote, side, qty)
	}

	if err != nil {
		return takerFillQuote{}, err
	}

	price := fill.Price
	filledQty := qty

	if fill.DepthCoverage <= 0 {
		filledQty = 0
	}

	if fill.DepthCoverage > 0 && fill.DepthCoverage < 1 {
		filledQty = qty * fill.DepthCoverage

		// The order shrinks to the covered quantity, so it pays the covered
		// book-walk price — not the blend that assumed an optimistic half-spread
		// fill for the remainder it no longer takes.
		if fill.PriceCovered > 0 {
			price = fill.PriceCovered
		}
	}

	if action == reasoning.ActionTakeProfit && side == trading.Sell && capPrice > 0 && price > capPrice {
		price = capPrice
	}

	return takerFillQuote{
		price:         price,
		filledQty:     filledQty,
		depthCoverage: fill.DepthCoverage,
	}, nil
}

func (orders *Orders) rejectCrossingPostOnly(
	params trading.AddParams,
	orderID string,
	quote broker.Quote,
) bool {
	limitPrice := params.LimitPrice

	if limitPrice <= 0 {
		orders.notifyFill(FillNotice{
			Params: params, OrderID: orderID,
			Reason: fmt.Sprintf("paper fill: post-only limit missing price for %s", params.Symbol),
		})

		return true
	}

	if !broker.WouldCrossPostOnly(quote, params.Side, limitPrice) {
		return false
	}

	orders.notifyFill(FillNotice{
		Params: params, OrderID: orderID,
		Reason: fmt.Sprintf("preflight: post-only limit would cross for %s", params.Symbol),
	})

	return true
}
