package manifold

import "math"

/*
roundedQuantity carries a computed quantity and a deterministic absolute bound
on rounding accumulated from its binary64 additions and subtractions.
*/
type roundedQuantity struct {
	value    float64
	roundoff float64
}

func (quantity roundedQuantity) Add(other roundedQuantity) roundedQuantity {
	if quantity.value == 0 {
		return roundedQuantity{
			value:    other.value,
			roundoff: upwardBound(quantity.roundoff, other.roundoff),
		}
	}

	if other.value == 0 {
		return roundedQuantity{
			value:    quantity.value,
			roundoff: upwardBound(quantity.roundoff, other.roundoff),
		}
	}

	return quantity.combine(other, quantity.value+other.value)
}

func (quantity roundedQuantity) Subtract(other roundedQuantity) roundedQuantity {
	if quantity.value == 0 {
		return roundedQuantity{
			value:    -other.value,
			roundoff: upwardBound(quantity.roundoff, other.roundoff),
		}
	}

	if other.value == 0 {
		return roundedQuantity{
			value:    quantity.value,
			roundoff: upwardBound(quantity.roundoff, other.roundoff),
		}
	}

	return quantity.combine(other, quantity.value-other.value)
}

func (quantity roundedQuantity) combine(
	other roundedQuantity,
	value float64,
) roundedQuantity {
	roundoff := upwardBound(quantity.roundoff, other.roundoff)
	roundoff = upwardBound(roundoff, roundingRadius(value))

	return roundedQuantity{value: value, roundoff: roundoff}
}

/*
roundingRadius is half the wider adjacent-float spacing around a rounded result.
At binade boundaries the two spacings differ, so using only one ULP can understate
the error. Subnormal half-ULPs are rounded upward to the smallest float.
*/
func roundingRadius(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return math.Inf(1)
	}

	upper := math.Nextafter(value, math.Inf(1))
	lower := math.Nextafter(value, math.Inf(-1))
	upperSpacing := math.Abs(upper - value)
	lowerSpacing := math.Abs(value - lower)
	spacing := finiteMaximum(upperSpacing, lowerSpacing)
	radius := spacing / 2

	if radius == 0 && spacing > 0 {
		return math.SmallestNonzeroFloat64
	}

	return radius
}

func finiteMaximum(left, right float64) float64 {
	leftFinite := !math.IsInf(left, 0) && !math.IsNaN(left)
	rightFinite := !math.IsInf(right, 0) && !math.IsNaN(right)

	if !leftFinite {
		return right
	}

	if !rightFinite {
		return left
	}

	return max(left, right)
}

/*
upwardBound adds nonnegative error terms and rounds the bound outward.
*/
func upwardBound(left, right float64) float64 {
	if left < 0 || right < 0 || math.IsNaN(left) || math.IsNaN(right) {
		return math.Inf(1)
	}

	if left == 0 {
		return right
	}

	if right == 0 {
		return left
	}

	bound := left + right

	if math.IsInf(bound, 0) {
		return math.Inf(1)
	}

	return math.Nextafter(bound, math.Inf(1))
}
