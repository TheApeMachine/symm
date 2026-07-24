package manifold

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
midpointDivisor avoids rebuilding the immutable fixed-point denominator for
every touch observation. Decimal.Div reads and internally aligns its operand.
*/
var midpointDivisor = decimal.NewFromInt64(2)

/*
marketTouch is the executable bid/ask boundary from one L3 sample. Decimal
scales are aligned before arithmetic so receiver-scale rounding cannot erase
price or quantity precision.
*/
type marketTouch struct {
	bidPrice      float64
	askPrice      float64
	bidPriceMoney *decimal.Decimal
	askPriceMoney *decimal.Decimal
	bidQuantity   *decimal.Decimal
	askQuantity   *decimal.Decimal
}

/*
scales returns touch midpoint, relative width, and side notionals for State.
*/
func (touch marketTouch) scales() (
	reference *decimal.Decimal,
	spread float64,
	buyCapacity *decimal.Decimal,
	sellCapacity *decimal.Decimal,
	ok bool,
) {
	if touch.bidPrice <= 0 || touch.askPrice <= touch.bidPrice ||
		touch.bidPriceMoney == nil || touch.askPriceMoney == nil ||
		touch.bidQuantity == nil || touch.askQuantity == nil ||
		touch.bidQuantity.Sign() <= 0 || touch.askQuantity.Sign() <= 0 {
		return nil, 0, nil, nil, false
	}

	reference = touch.reference()

	return reference,
		touch.spread(reference),
		touch.notional(touch.askPriceMoney, touch.askQuantity),
		touch.notional(touch.bidPriceMoney, touch.bidQuantity),
		true
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
