package adaptive

import (
	"errors"
	"math"
)

/*
SigmaClamp bounds a derived signal to a band around its exponentially
weighted mean using VarianceEWMA — a statistical bump stop that damps
pathological spikes while letting legitimate drift move the baseline
as observations accumulate.
*/
type SigmaClamp struct {
	moments EWMoments
	kSigma  float64
	minObs  int
	alpha   float64
	epsilon float64
}

/*
NewSigmaClamp builds a clamp with k-sigma bands; alpha drives EWMoments
updates on each pipeline step.
*/
func NewSigmaClamp(kSigma float64, minObs int, alpha float64) *SigmaClamp {
	if kSigma <= 0 {
		kSigma = 3
	}

	if minObs < 2 {
		minObs = 8
	}

	if alpha <= 0 || alpha > 1 {
		alpha = 0.0625
	}

	return &SigmaClamp{
		kSigma:  kSigma,
		minObs:  minObs,
		alpha:   alpha,
		epsilon: 1e-12,
	}
}

/*
Next ingests the previous dynamic’s output in `out`, compares it against
moments gathered *before* this sample so a single pathological spike cannot
widen the band in the same step, then folds the (possibly clamped) value
into the tracker.
*/
func (clamp *SigmaClamp) Next(
	out float64, values ...float64,
) (float64, error) {
	if clamp == nil {
		return 0, errors.New("adaptive: SigmaClamp.Next nil receiver")
	}

	_ = values

	folded := out

	if clamp.moments.Observations() >= clamp.minObs {
		std := math.Sqrt(clamp.moments.VarianceEWMA())

		if std >= clamp.epsilon {
			mean := clamp.moments.Mean()
			hi := mean + clamp.kSigma*std
			lo := mean - clamp.kSigma*std

			if out > hi {
				folded = hi
			}

			if out < lo {
				folded = lo
			}
		}
	}

	if err := clamp.moments.Update(folded, clamp.alpha); err != nil {
		return 0, err
	}

	return folded, nil
}

/*
Reset clears accumulated moments.
*/
func (clamp *SigmaClamp) Reset() error {
	if clamp == nil {
		return errors.New("adaptive: SigmaClamp.Reset nil receiver")
	}

	clamp.moments.Reset()

	return nil
}

/*
Clone returns an independent SigmaClamp with copied moments and parameters.
*/
func (clamp *SigmaClamp) Clone() *SigmaClamp {
	if clamp == nil {
		return nil
	}

	return &SigmaClamp{
		moments: clamp.moments,
		kSigma:  clamp.kSigma,
		minObs:  clamp.minObs,
		alpha:   clamp.alpha,
		epsilon: clamp.epsilon,
	}
}
