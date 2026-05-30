package adaptive

import (
	"errors"
	"math"
	"sync"
)

const (
	// defaultSNRMinObs is how many samples a series must accumulate before a
	// noise floor is trusted; until then SNR is reported as 0.
	defaultSNRMinObs = 12
	// defaultSNRAlpha is the EW update rate of the baseline (~20-sample memory).
	defaultSNRAlpha = 0.05
	// defaultSNRClampSigma bounds the value folded into the baseline so a single
	// spike — the very thing we detect — cannot inflate the floor that detects it.
	defaultSNRClampSigma = 4
	// snrEpsilon guards against a degenerate (zero-spread) noise estimate.
	snrEpsilon = 1e-12
)

/*
SNR is the detection-theory signal-to-noise ratio of one scalar series: how many
of the series' own noise standard deviations the latest value stands above its
running mean. It is dimensionless and therefore comparable across signals that
measure entirely different raw quantities — a value of 1 means "one sigma above
my own background" whether the input was order-book odds, a peer ratio, or a
trade intensity.

The mean and standard deviation are exponentially weighted, measured from the
samples seen *before* the current one (so the current reading is scored against
its history, not itself), and the folded value is clamped to clampSigma so a
genuine spike registers as a high SNR without raising the floor.
*/
type SNR struct {
	moments    EWMoments
	minObs     int
	alpha      float64
	clampSigma float64
}

/*
NewSNR builds an SNR tracker with the default baseline memory and robustness.
*/
func NewSNR() *SNR {
	return &SNR{
		minObs:     defaultSNRMinObs,
		alpha:      defaultSNRAlpha,
		clampSigma: defaultSNRClampSigma,
	}
}

/*
Score folds value into the running baseline and returns its SNR: max(0, (value −
mean) / std) against the baseline from before this sample. It returns 0 while the
series is still warming up or has no measurable spread.
*/
func (snr *SNR) Score(value float64) float64 {
	result := 0.0
	folded := value

	if snr.moments.Observations() >= snr.minObs {
		std := math.Sqrt(snr.moments.VarianceEWMA())

		if std >= snrEpsilon {
			mean := snr.moments.Mean()

			if zScore := (value - mean) / std; zScore > 0 {
				result = zScore
			}

			folded = clampToBand(value, mean, snr.clampSigma*std)
		}
	}

	// alpha is fixed-valid by the constructor, so Update cannot error here.
	_ = snr.moments.Update(folded, snr.alpha)

	return result
}

/*
Next adapts SNR to the numeric pipeline interface: it scores the previous stage's
output as the raw strength.
*/
func (snr *SNR) Next(out float64, _ ...float64) (float64, error) {
	if snr == nil {
		return 0, errors.New("adaptive: SNR.Next nil receiver")
	}

	return snr.Score(out), nil
}

// clampToBand limits value to [center-radius, center+radius].
func clampToBand(value, center, radius float64) float64 {
	if value > center+radius {
		return center + radius
	}

	if value < center-radius {
		return center - radius
	}

	return value
}

/*
SNRField keeps an independent SNR baseline per symbol, so one signal that scores
many symbols normalizes each against its own history. It is the per-symbol form
of SNR for cross-sectional signals.
*/
type SNRField struct {
	mu     sync.Mutex
	series map[string]*SNR
}

/*
NewSNRField builds an empty per-symbol SNR field.
*/
func NewSNRField() *SNRField {
	return &SNRField{series: make(map[string]*SNR)}
}

/*
Score returns the SNR of value within symbol's own series, creating the series on
first use.
*/
func (field *SNRField) Score(symbol string, value float64) float64 {
	field.mu.Lock()
	tracker, ok := field.series[symbol]

	if !ok {
		tracker = NewSNR()
		field.series[symbol] = tracker
	}

	field.mu.Unlock()

	return tracker.Score(value)
}
