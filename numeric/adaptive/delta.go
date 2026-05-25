package adaptive

/*
Delta tracks the rate of change of a signal. It
measures how fast the signal is moving by comparing
each observation to the previous one. The raw delta
is smoothed by an EMA so that a single spike does
not dominate — the EMA itself derives its rate from
the delta's own volatility, so no configuration is
needed.
*/
type Delta struct {
	smoother *EMA
	previous float64
}

/*
NewDelta creates a new Delta. The internal smoothing
is handled by an EMA that bootstraps itself.
*/
func NewDelta(initial float64) *Delta {
	return &Delta{
		smoother: NewEMA(initial),
		previous: initial,
	}
}

/*
Next accepts the current signal value and returns
the smoothed rate of change. On the first observation
it returns zero, since there is no previous value
to compare against.
*/
func (delta *Delta) Next(
	out float64, values ...float64,
) (change float64, err error) {
	for _, observation := range values {
		rawChange := observation - delta.previous
		delta.previous = observation

		change, err = delta.smoother.Next(out, rawChange)

		if err != nil {
			return 0, err
		}

		out = change
	}

	return change, nil
}

/*
Reset clears the Delta back to its unobserved state.
*/
func (delta *Delta) Reset() error {
	delta.previous = 0
	return delta.smoother.Reset()
}
