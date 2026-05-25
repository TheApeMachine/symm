package adaptive

/*
Accumulator is a capacitor. It charges on positive
signal, drains on negative signal, and holds on
neutral signal. The amount it charges or drains is
proportional to the signal strength — stronger
signals move it faster.

There are no bounds. If the system drives it up
forever, that is signal. If it oscillates, that is
signal. The Accumulator just integrates what it
receives. If bounds are needed, the system must
produce counterforce — that is the caller's
responsibility through composition with other
dynamics.
*/
type Accumulator struct {
	value    float64
	smoother *EMA
}

/*
NewAccumulator creates a new Accumulator starting
at zero. The incoming signal is smoothed by an EMA
before accumulation so that noise does not dominate
the integral.
*/
func NewAccumulator(initial float64) *Accumulator {
	return &Accumulator{
		value:    initial,
		smoother: NewEMA(initial),
	}
}

/*
Next accepts a signal and accumulates it. Positive
values charge, negative values drain, zero holds.
Returns the current accumulated level.
*/
func (accumulator *Accumulator) Next(
	_out float64, values ...float64,
) (result float64, err error) {
	for _, observation := range values {
		smoothed, err := accumulator.smoother.Next(0, observation)

		if err != nil {
			return 0, err
		}

		accumulator.value += smoothed
	}

	return accumulator.value, nil
}

/*
Reset clears the Accumulator back to zero and
resets the internal smoother.
*/
func (accumulator *Accumulator) Reset() error {
	accumulator.value = 0
	return accumulator.smoother.Reset()
}
