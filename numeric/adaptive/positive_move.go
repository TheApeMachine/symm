package adaptive

import "fmt"

/*
PositiveMove maps upward relative change (current/anchor - 1) into (0, 1].
Below bound the score ramps linearly; above bound excess uses (x-1)/x on the
scaled ratio. Point-in-time: no internal smoothing.
*/
type PositiveMove struct {
	bound float64
}

/*
NewPositiveMove creates a positive-move scorer with the given linear ramp bound.
*/
func NewPositiveMove(bound float64) *PositiveMove {
	return &PositiveMove{bound: bound}
}

/*
Next accepts current price and anchor reference.
*/
func (move *PositiveMove) Next(out float64, values ...float64) (float64, error) {
	_ = out

	if len(values) != 2 {
		return 0, fmt.Errorf(
			"adaptive: PositiveMove.Next expects current and anchor, got %d values",
			len(values),
		)
	}

	if move.bound <= 0 {
		return 0, fmt.Errorf("adaptive: PositiveMove bound must be positive")
	}

	current := values[0]
	anchor := values[1]

	if anchor <= 0 || current <= anchor {
		return 0, nil
	}

	raw := (current - anchor) / anchor
	scaled := raw / move.bound

	if scaled <= 1 {
		return scaled, nil
	}

	return (scaled - 1) / scaled, nil
}

/*
Reset is a no-op for PositiveMove.
*/
func (move *PositiveMove) Reset() error {
	return nil
}
