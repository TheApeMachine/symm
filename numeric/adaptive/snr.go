package adaptive

import (
	"errors"
	"fmt"
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
running mean. Signals pass category standout — winner margin over alternatives —
not band clarity (Confidence).

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
series is still warming up. Invalid standout or an immeasurable noise floor returns
an error — never a silent substitute.
*/
func (snr *SNR) Score(value float64) (float64, error) {
	if err := validateStandout(value); err != nil {
		return 0, err
	}

	result := 0.0
	folded := value

	if snr.moments.Observations() >= snr.minObs {
		mean := snr.moments.Mean()
		std := math.Sqrt(snr.moments.VarianceEWMA())

		if std < snrEpsilon {
			if value != mean {
				return 0, ErrInsufficientStandoutSpread
			}
		} else if zScore := (value - mean) / std; zScore > 0 {
			result = zScore
			folded = clampToBand(value, mean, snr.clampSigma*std)
		}
	}

	if err := snr.moments.Update(folded, snr.alpha); err != nil {
		return 0, err
	}

	return result, nil
}

var ErrInsufficientStandoutSpread = errors.New("adaptive: SNR insufficient standout spread")

func validateStandout(value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return fmt.Errorf("adaptive: SNR invalid standout: %v", value)
	}

	if value > 1 {
		return fmt.Errorf("adaptive: SNR standout above unit band: %v", value)
	}

	return nil
}

/*
Next adapts SNR to the numeric pipeline interface: it scores the previous stage's
output as the raw strength.
*/
func (snr *SNR) Next(out float64, _ ...float64) (float64, error) {
	if snr == nil {
		return 0, errors.New("adaptive: SNR.Next nil receiver")
	}

	return snr.Score(out)
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
func (field *SNRField) Score(symbol string, value float64) (float64, error) {
	field.mu.Lock()
	tracker, ok := field.series[symbol]

	if !ok {
		tracker = NewSNR()
		field.series[symbol] = tracker
	}

	field.mu.Unlock()

	return tracker.Score(value)
}
