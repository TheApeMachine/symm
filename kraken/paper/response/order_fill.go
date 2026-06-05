package response

import (
	"fmt"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
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
func (orders *Orders) takerFillPrice(
	symbol string,
	side trading.Side,
	qty float64,
	capPrice float64,
	action perspectives.ActionType,
) (price float64, err error) {
	quote, ok := orders.quotes.Snapshot(symbol)

	if !ok {
		return 0, fmt.Errorf("paper fill: no quote for %s", symbol)
	}

	fill, err := broker.SlippageFill(quote, side, qty)

	if err != nil {
		return 0, err
	}

	price = fill.Price

	if action == perspectives.ActionTakeProfit && side == trading.Sell && capPrice > 0 && price > capPrice {
		price = capPrice
	}

	return price, nil
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
