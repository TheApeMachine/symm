package response

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
)

type restingTriggeredOrder struct {
	params       trading.AddParams
	triggerPrice float64
	peakPrice    float64
	trailing     bool
	trailOffset  float64
}

func (orders *Orders) parkTriggeredOrder(params trading.AddParams) {
	triggerPrice, referencePrice, trailOffset, trailing, immediate := orders.resolveTriggeredOrder(params)

	if immediate {
		orders.fillMarket(params)
		return
	}

	orders.restingTriggered[params.ClOrdID] = restingTriggeredOrder{
		params:       params,
		triggerPrice: triggerPrice,
		peakPrice:    referencePrice,
		trailing:     trailing,
		trailOffset:  trailOffset,
	}
	orders.model[params.ClOrdID] = trading.OrderUpdate{
		OrderID: params.ClOrdID,
	}
}

func (orders *Orders) EvaluateTicker(ticker *market.TickerUpdate) {
	if orders == nil || ticker == nil || ticker.Symbol == "" {
		return
	}

	for clOrdID, resting := range orders.restingTriggered {
		if resting.params.Symbol != ticker.Symbol {
			continue
		}

		markPrice, markErr := triggeredMarkPrice(ticker, resting.params.Side)

		if markErr != nil {
			errnie.Error(markErr)
			continue
		}

		resting = orders.ratchetTrailing(resting, markPrice)

		if !triggeredOrderCrossed(resting, markPrice) {
			orders.restingTriggered[clOrdID] = resting
			continue
		}

		orders.fillMarket(resting.params)
		delete(orders.restingTriggered, clOrdID)
		delete(orders.model, clOrdID)
	}
}

func (orders *Orders) resolveTriggeredOrder(
	params trading.AddParams,
) (
	triggerPrice float64,
	referencePrice float64,
	trailOffset float64,
	trailing bool,
	immediate bool,
) {
	referencePrice, referenceErr := orders.referencePrice(params.Symbol, params.Side)

	if referenceErr != nil {
		errnie.Error(referenceErr)
		return 0, 0, 0, false, true
	}

	switch params.OrderType {
	case trading.TrailingStop, trading.TrailingStopLimit:
		trailing = true
		trailOffset = triggerOffset(params)

		if trailOffset <= 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"paper orders: trailing stop trigger offset is required",
				errnie.Require(map[string]any{
					"cl_ord_id":  params.ClOrdID,
					"order_type": params.OrderType,
				}),
			))

			return 0, referencePrice, 0, true, true
		}

		return referencePrice * (1 - trailOffset), referencePrice, trailOffset, true, false
	case trading.TakeProfit, trading.TakeProfitLimit:
		triggerPrice = triggerAbsolutePrice(params, referencePrice, trading.Buy)

		if triggerPrice <= 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"paper orders: take-profit trigger price is required",
				errnie.Require(map[string]any{
					"cl_ord_id": params.ClOrdID,
					"symbol":    params.Symbol,
				}),
			))

			return 0, referencePrice, 0, false, true
		}

		return triggerPrice, referencePrice, 0, false, false
	case trading.StopLoss, trading.StopLossLimit:
		triggerPrice = triggerAbsolutePrice(params, referencePrice, trading.Sell)

		if triggerPrice <= 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"paper orders: stop-loss trigger price is required",
				errnie.Require(map[string]any{
					"cl_ord_id": params.ClOrdID,
					"symbol":    params.Symbol,
				}),
			))

			return 0, referencePrice, 0, false, true
		}

		return triggerPrice, referencePrice, 0, false, false
	default:
		errnie.Error(errnie.Err(
			errnie.Validation,
			"paper orders: unsupported triggered order type",
			errnie.Require(map[string]any{
				"cl_ord_id":  params.ClOrdID,
				"order_type": params.OrderType,
			}),
		))

		return 0, referencePrice, 0, false, true
	}
}

func (orders *Orders) referencePrice(
	symbol string,
	side trading.Side,
) (float64, error) {
	if orders.catalog == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"paper orders: pair catalog is required for trigger reference",
			errnie.Require(map[string]any{
				"symbol": symbol,
				"side":   side,
			}),
		))
	}

	book, bookErr := orders.catalog.DepthForSymbol(symbol, orders.bookDepthLevels)

	if bookErr != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"paper orders: depth for trigger reference",
			bookErr,
		))
	}

	if side == trading.Sell {
		return bestDepthPrice(book.Bids)
	}

	return bestDepthPrice(book.Asks)
}

func bestDepthPrice(levels [][]any) (float64, error) {
	if len(levels) == 0 || len(levels[0]) == 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"paper orders: depth reference is missing",
			errnie.Require(map[string]any{
				"levels": len(levels),
			}),
		))
	}

	price, priceErr := depthFloat(levels[0][0])

	if priceErr != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"paper orders: depth reference price",
			priceErr,
		))
	}

	return price, nil
}

func triggerOffset(params trading.AddParams) float64 {
	if params.Triggers == nil || params.Triggers.Price <= 0 {
		return 0
	}

	if params.Triggers.PriceType == "pct" {
		return params.Triggers.Price
	}

	return params.Triggers.Price / 100
}

func triggerAbsolutePrice(
	params trading.AddParams,
	referencePrice float64,
	direction trading.Side,
) float64 {
	if params.Triggers == nil || params.Triggers.Price <= 0 {
		return params.LimitPrice
	}

	if params.Triggers.PriceType == "pct" {
		offset := params.Triggers.Price

		if direction == trading.Buy {
			return referencePrice * (1 + offset)
		}

		return referencePrice * (1 - offset)
	}

	return params.Triggers.Price
}

func (orders *Orders) ratchetTrailing(
	resting restingTriggeredOrder,
	markPrice float64,
) restingTriggeredOrder {
	if !resting.trailing || markPrice <= resting.peakPrice {
		return resting
	}

	resting.peakPrice = markPrice
	resting.triggerPrice = markPrice * (1 - resting.trailOffset)

	return resting
}

func triggeredOrderCrossed(
	resting restingTriggeredOrder,
	markPrice float64,
) bool {
	switch resting.params.OrderType {
	case trading.TakeProfit, trading.TakeProfitLimit:
		return markPrice >= resting.triggerPrice
	case trading.StopLoss, trading.StopLossLimit,
		trading.TrailingStop, trading.TrailingStopLimit:
		return markPrice <= resting.triggerPrice
	default:
		return false
	}
}

func triggeredMarkPrice(
	ticker *market.TickerUpdate,
	side trading.Side,
) (float64, error) {
	if ticker == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"paper orders: ticker is required for trigger evaluation",
			nil,
		))
	}

	if side == trading.Sell {
		if ticker.Bid > 0 {
			return ticker.Bid, nil
		}

		if ticker.Last > 0 {
			return ticker.Last, nil
		}

		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"paper orders: sell trigger requires bid or last",
			errnie.Require(map[string]any{
				"symbol": ticker.Symbol,
			}),
		))
	}

	if ticker.Ask > 0 {
		return ticker.Ask, nil
	}

	if ticker.Last > 0 {
		return ticker.Last, nil
	}

	return 0, errnie.Error(errnie.Err(
		errnie.Validation,
		"paper orders: buy trigger requires ask or last",
		errnie.Require(map[string]any{
			"symbol": ticker.Symbol,
		}),
	))
}
