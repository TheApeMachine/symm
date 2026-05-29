package adaptive

/*
FracDiff is a fixed-width fractional differencing filter.

Integer differencing (today's price minus yesterday's) makes a series stationary but throws away
its long memory: the differenced series has no idea the asset has been grinding against a wall for
hours. Fractional differencing instead applies the binomial expansion of the difference operator
(1-L)^d for a real order d in (0, 1), a decaying tail of weights rather than a single subtraction.
With a small d the result passes a stationarity test while preserving most of the level's memory —
the razor's edge the order parameter rides. Feeding a fractionally differenced price into the fluid
field gives a turbulence reading that reflects genuine microstructural chaos instead of the artifact
of a widening spread during a quiet, low-volume window.

The binomial weights are w_0 = 1, w_k = -w_{k-1} * (d - k + 1) / k, applied to the most recent
`width` observations with w_0 multiplying the newest.
*/
type FracDiff struct {
	weights []float64
	buffer  []float64
	width   int
	head    int
	count   int
}

/*
NewFracDiff builds a filter of the given fractional order and window width. A non-positive width
defaults to one; the order may be any real value (0 < d < 1 is the useful range for price series).
*/
func NewFracDiff(order float64, width int) *FracDiff {
	if width < 1 {
		width = 1
	}

	weights := make([]float64, width)
	weights[0] = 1

	for k := 1; k < width; k++ {
		weights[k] = -weights[k-1] * (order - float64(k) + 1) / float64(k)
	}

	return &FracDiff{
		weights: weights,
		buffer:  make([]float64, width),
		width:   width,
		head:    -1,
	}
}

/*
Push folds one observation and returns the fractionally differenced value. The second result is
false until the window has filled, so callers never act on a half-warmed filter.
*/
func (fracDiff *FracDiff) Push(observation float64) (float64, bool) {
	fracDiff.head = (fracDiff.head + 1) % fracDiff.width
	fracDiff.buffer[fracDiff.head] = observation

	if fracDiff.count < fracDiff.width {
		fracDiff.count++
	}

	if fracDiff.count < fracDiff.width {
		return 0, false
	}

	sum := 0.0
	index := fracDiff.head

	for k := 0; k < fracDiff.width; k++ {
		sum += fracDiff.weights[k] * fracDiff.buffer[index]
		index = (index - 1 + fracDiff.width) % fracDiff.width
	}

	return sum, true
}

/*
Warm reports whether the filter has seen enough observations to emit a value.
*/
func (fracDiff *FracDiff) Warm() bool {
	return fracDiff.count >= fracDiff.width
}

/*
Reset clears the buffered observations, keeping the precomputed weights.
*/
func (fracDiff *FracDiff) Reset() {
	fracDiff.head = -1
	fracDiff.count = 0

	for index := range fracDiff.buffer {
		fracDiff.buffer[index] = 0
	}
}
