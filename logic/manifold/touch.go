package manifold

import (
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
midpointDivisor avoids rebuilding the immutable fixed-point denominator for
every touch observation. Decimal.Div reads and internally aligns its operand.
*/
var midpointDivisor = decimal.NewFromInt64(2)

/*
marketTouch owns the exact executable bid/ask boundary derived from the L3
population. It aligns independent Kraken decimal scales before arithmetic so
receiver-scale rounding cannot erase price or quantity precision.
*/
type marketTouch struct {
	bidPrice      float64
	askPrice      float64
	bidPriceMoney *decimal.Decimal
	askPriceMoney *decimal.Decimal
	bidQuantity   *decimal.Decimal
	askQuantity   *decimal.Decimal
	bidOrders     int
	askOrders     int
}

/*
reference returns the exact touch midpoint at its derived decimal scale.
*/
func (touch marketTouch) reference() *decimal.Decimal {
	scale := max(
		touch.bidPriceMoney.GetScale(),
		touch.askPriceMoney.GetScale(),
	) + 1

	return touch.bidPriceMoney.SetScale(scale).
		Add(touch.askPriceMoney).
		Div(midpointDivisor)
}

/*
spread computes exact touch width/reference before the dimensionless boundary.
*/
func (touch marketTouch) spread(reference *decimal.Decimal) float64 {
	priceScale := max(
		touch.askPriceMoney.GetScale(),
		touch.bidPriceMoney.GetScale(),
	)
	width := touch.askPriceMoney.SetScale(priceScale).
		Sub(touch.bidPriceMoney)
	scale := max(
		int64(decimal.DefaultScale),
		width.GetScale(),
		reference.GetScale(),
	)

	return width.SetScale(scale).Div(reference).Float64()
}

/*
notional multiplies exact fixed-point price and quantity at their combined
scale. Integer products retain one fractional place because the SDK's
scale-zero banker rounding misclassifies exact odd integers as half-way values.
*/
func (touch marketTouch) notional(
	price *decimal.Decimal,
	quantity *decimal.Decimal,
) *decimal.Decimal {
	scale := max(int64(1), price.GetScale()+quantity.GetScale())

	return price.SetScale(scale).Mul(quantity)
}

/*
observe updates one side of the best touch and accumulates its exact quantity.
*/
func (touch *marketTouch) observe(order physicalOrder) {
	if order.side == book.Bid {
		touch.bidOrders++
	}

	if order.side == book.Ask {
		touch.askOrders++
	}

	if order.side == book.Bid && order.price >= touch.bidPrice {
		if order.price > touch.bidPrice {
			touch.bidPrice = order.price
			touch.bidPriceMoney = order.priceMoney
			touch.bidQuantity = decimal.NewFromInt64(0)
		}

		if order.quantityMoney.GetScale() > touch.bidQuantity.GetScale() {
			touch.bidQuantity, order.quantityMoney = order.quantityMoney, touch.bidQuantity
		}

		touch.bidQuantity = touch.bidQuantity.Add(order.quantityMoney)
	}

	if order.side != book.Ask ||
		(touch.askPrice != 0 && order.price > touch.askPrice) {
		return
	}

	if touch.askPrice == 0 || order.price < touch.askPrice {
		touch.askPrice = order.price
		touch.askPriceMoney = order.priceMoney
		touch.askQuantity = decimal.NewFromInt64(0)
	}

	if order.quantityMoney.GetScale() > touch.askQuantity.GetScale() {
		touch.askQuantity, order.quantityMoney = order.quantityMoney, touch.askQuantity
	}

	touch.askQuantity = touch.askQuantity.Add(order.quantityMoney)
}
