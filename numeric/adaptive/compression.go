package adaptive

import "fmt"

/*
Compression scores how much lower the current observation is versus baseline.
Used for spread tightening: smaller spread versus its baseline yields a higher score.
*/
type Compression struct {
	smoother *EMA
}

/*
NewCompression creates a compression dynamic with an internal EMA smoother.
*/
func NewCompression(rate float64) *Compression {
	return &Compression{smoother: NewEMA(rate)}
}

/*
Next accepts current and baseline. Returns smoothed baseline/current when current
is below baseline, otherwise zero.
*/
func (compression *Compression) Next(
	out float64, values ...float64,
) (float64, error) {
	var current float64
	var baseline float64

	switch len(values) {
	case 1:
		if out <= 0 {
			return 0, nil
		}

		current = values[0]
		baseline = out
	case 2:
		current = values[0]
		baseline = values[1]
	default:
		return 0, fmt.Errorf(
			"adaptive: Compression.Next expects one or two values, got %d",
			len(values),
		)
	}

	if current <= 0 || baseline <= 0 || current >= baseline {
		return 0, nil
	}

	return compression.smoother.Next(out, baseline/current)
}

/*
Reset clears the internal smoother.
*/
func (compression *Compression) Reset() error {
	return compression.smoother.Reset()
}
