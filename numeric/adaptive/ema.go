package adaptive

import "math"

/*
EMA (Exponential Moving Average) gives more weight to
recent observations. It bootstraps from the first
observation and derives its own smoothing rate from
the signal's volatility — no configuration needed.

When the signal is volatile, the EMA reacts faster.
When the signal is stable, the EMA smooths harder.
The rate itself is an EMA of the absolute delta,
relative to the observed range, so it is fully
derived from the data.
*/
type EMA struct {
	value     float64
	rate      float64
	meanDelta float64
	observed  bool
	min       float64
	max       float64
	count     int
}

/*
NewEMA creates a new EMA. No initial value or rate is
needed — both are derived from the first observations.
*/
func NewEMA(rate float64) *EMA {
	return &EMA{
		rate: rate,
	}
}

/*
Next absorbs the incoming values and returns the
current smoothed state. On the first observation,
the EMA simply adopts the value. From the second
observation onward, the rate is derived from the
signal's own volatility relative to its observed
range.
*/
func (ema *EMA) observations(out float64, values []float64) ([]float64, bool) {
	if out == 0 && len(values) == 2 {
		return nil, false
	}

	if out > 0 && len(values) > 1 {
		return []float64{out}, true
	}

	if len(values) == 0 {
		if out <= 0 {
			return nil, false
		}

		return []float64{out}, true
	}

	return values, true
}

func (ema *EMA) Next(out float64, values ...float64) (float64, error) {
	observations, ok := ema.observations(out, values)

	if !ok {
		return 0, nil
	}

	for _, observation := range observations {
		if !ema.observed {
			ema.value = observation
			ema.min = observation
			ema.max = observation
			ema.observed = true
			continue
		}

		// Track the observed range of the signal.
		if observation < ema.min {
			ema.min = observation
		}

		if observation > ema.max {
			ema.max = observation
		}

		// Derive the rate from how volatile the signal
		// is relative to its own range. The delta tells
		// us how much the signal just moved. The range
		// tells us how much it has ever moved. The ratio
		// is our natural smoothing rate.
		delta := observation - ema.value
		if delta < 0 {
			delta = -delta
		}

		// Smooth the delta itself so a single spike
		// does not whip the rate around.
		ema.count++
		ema.meanDelta += (delta - ema.meanDelta) / float64(ema.count)

		spread := ema.max - ema.min

		if spread > 0 {
			ema.rate = ema.meanDelta / spread
			ema.rate = math.Max(0, math.Min(1, ema.rate))
		} else {
			ema.rate = 0
		}

		ema.value = ema.rate*observation + (1-ema.rate)*ema.value
	}

	return ema.value, nil
}

/*
Value returns the last smoothed output without pushing a new observation.
*/
func (ema *EMA) Value() float64 {
	return ema.value
}

/*
Reset clears the EMA back to its unobserved state,
ready to bootstrap from the next observation.
*/
func (ema *EMA) Reset() error {
	ema.value = 0
	ema.rate = 0
	ema.meanDelta = 0
	ema.observed = false
	ema.min = 0
	ema.max = 0
	return nil
}

/*
Clone returns an independent EMA with the same internal state. Used when
snapshotting numeric.Derived chains so callers cannot mutate the original
via Dynamic.Next.
*/
func (ema *EMA) Clone() *EMA {
	if ema == nil {
		return nil
	}

	return &EMA{
		value:     ema.value,
		rate:      ema.rate,
		meanDelta: ema.meanDelta,
		observed:  ema.observed,
		min:       ema.min,
		max:       ema.max,
		count:     ema.count,
	}
}
