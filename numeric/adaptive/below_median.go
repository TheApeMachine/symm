package adaptive

import "sort"

/*
BelowMedian passes out through when it is strictly below the cross-section median
of out and all peer values. With fewer than two values out passes unchanged.
*/
type BelowMedian struct{}

/*
NewBelowMedian creates a cross-section median gate.
*/
func NewBelowMedian() *BelowMedian {
	return &BelowMedian{}
}

/*
Next compares out against the median of out and values.
*/
func (gate *BelowMedian) Next(out float64, values ...float64) (float64, error) {
	if out <= 0 {
		return 0, nil
	}

	sample := append([]float64{out}, values...)

	if len(sample) < 2 {
		return out, nil
	}

	if out >= crossSectionMedian(sample) {
		return 0, nil
	}

	return out, nil
}

func crossSectionMedian(sample []float64) float64 {
	sorted := append([]float64(nil), sample...)
	sort.Float64s(sorted)

	mid := len(sorted) / 2

	if len(sorted)%2 == 1 {
		return sorted[mid]
	}

	return (sorted[mid-1] + sorted[mid]) / 2
}

/*
Reset is a no-op for BelowMedian.
*/
func (gate *BelowMedian) Reset() error {
	return nil
}
