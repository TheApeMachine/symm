package adaptive

import "fmt"

/*
RelativeMove returns the smoothed relative change (current/baseline - 1).
Positive values mean price rose versus the reference; negative means it fell.
*/
type RelativeMove struct {
	ratio *Ratio
}

/*
NewRelativeMove creates a relative move dynamic backed by Ratio smoothing.
*/
func NewRelativeMove() *RelativeMove {
	return &RelativeMove{ratio: NewRatio(0)}
}

/*
Next accepts current and baseline as the two operands to Ratio.
*/
func (move *RelativeMove) Next(out float64, values ...float64) (float64, error) {
	if len(values) != 2 {
		return 0, fmt.Errorf(
			"adaptive: RelativeMove.Next expects current and baseline, got %d values",
			len(values),
		)
	}

	ratio, err := move.ratio.Next(out, values...)

	if err != nil {
		return 0, err
	}

	return ratio - 1, nil
}

/*
Reset clears the internal ratio smoother.
*/
func (move *RelativeMove) Reset() error {
	return move.ratio.Reset()
}
