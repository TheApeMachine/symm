package data

import (
	"errors"
	"iter"
	"math"
	"strconv"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
QualityFacts are the optional estimator fields Finalize reads. Absence is not
zero: a missing support is a direct measurement, not N=0.
*/
type QualityFacts struct {
	Support        float64
	Divergence     float64
	NoiseVariance  float64
	MahalanobisSNR float64
	Maturity       float64
	HasSupport     bool
	HasDivergence  bool
	HasNoise       bool
	HasMahalanobis bool
	HasMaturity    bool
}

/*
QualityReading is derived maturity/SNR. SNRDefined distinguishes a measured
zero from an unestimable noise model.
*/
type QualityReading struct {
	SNR        float64
	SNRDefined bool
	Estimated  bool
	Maturity   float64
}

/*
Quality owns that derivation. Support>1 is required before Mahalanobis
overrides scalar SNR.
*/
type Quality struct {
	err error
	out QualityReading
}

func NewQuality() core.Primitive {
	return &Quality{}
}

func (op *Quality) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			facts := (*QualityFacts)(arriving)
			reading := QualityReading{
				Estimated: facts.HasSupport || facts.HasDivergence || facts.HasMahalanobis,
				Maturity:  1,
			}

			// Scalar and Mahalanobis SNR both require Support > 1 so a near-zero
			// noise estimate from an immature sample cannot publish astronomical
			// SNR. Inf/NaN are refused rather than clamped.
			if facts.HasSupport {
				reading.Maturity = 0

				if facts.Support > 1 {
					reading.Maturity = 1 - 1/facts.Support

					if facts.HasDivergence && facts.HasNoise &&
						distinguishableNoise(facts.NoiseVariance, facts.Divergence) {
						snr := facts.Divergence * facts.Divergence / facts.NoiseVariance
						bound := 1 / math.Sqrt(machineEpsilon)

						if !math.IsInf(snr, 0) && !math.IsNaN(snr) && snr < bound {
							reading.SNR = snr
							reading.SNRDefined = true
						}
					}

					// Mahalanobis SNR is refused when non-finite or when it exceeds the
					// float64 relative condition bound 1/sqrt(eps). Values beyond that
					// imply a noise floor below relative ULP — not an estimable SNR.
					if facts.HasMahalanobis && facts.MahalanobisSNR >= 0 &&
						!math.IsInf(facts.MahalanobisSNR, 0) && !math.IsNaN(facts.MahalanobisSNR) &&
						facts.MahalanobisSNR < 1/math.Sqrt(machineEpsilon) {
						reading.SNR = facts.MahalanobisSNR
						reading.SNRDefined = true
					}
				}
			}

			if !facts.HasSupport && facts.HasMaturity {
				reading.Maturity = facts.Maturity
			}

			op.out = reading

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Quality) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}


// machineEpsilon is float64 epsilon (math.Nextafter(1, 2) - 1).
const machineEpsilon = 2.220446049250313e-16

/*
distinguishableNoise reports whether a variance estimate is large enough,
relative to a reference magnitude, to support a float64 SNR = d²/v without
publishing astronomical values from a collapsed noise floor. The floor is
sqrt(eps)·max(1, |reference|) — the same relative distinguishability used by
Fisher/CausalResidual ScoreScale gates, raised to relative form so near-zero
processes cannot claim billion-scale SNR from sub-ULP variance.
*/
func distinguishableNoise(variance, reference float64) bool {
	if variance <= 0 || math.IsNaN(variance) || math.IsInf(variance, 0) {
		return false
	}

	ref := math.Abs(reference)
	if ref < 1 {
		ref = 1
	}

	return math.Sqrt(variance) > math.Sqrt(machineEpsilon)*ref
}

func factsFromMetadata(metadata map[string]string) QualityFacts {
	facts := QualityFacts{}

	if metadata == nil {
		return facts
	}

	if valueStr, ok := metadata[MetadataSupport]; ok {
		if val, err := strconv.ParseFloat(valueStr, 64); err == nil {
			facts.Support, facts.HasSupport = val, true
		}
	}

	if valueStr, ok := metadata[MetadataDivergence]; ok {
		if val, err := strconv.ParseFloat(valueStr, 64); err == nil {
			facts.Divergence, facts.HasDivergence = val, true
		}
	}

	if valueStr, ok := metadata[MetadataNoiseVariance]; ok {
		if val, err := strconv.ParseFloat(valueStr, 64); err == nil {
			facts.NoiseVariance, facts.HasNoise = val, true
		}
	}

	if valueStr, ok := metadata[MetadataMahalanobisSNR]; ok {
		if val, err := strconv.ParseFloat(valueStr, 64); err == nil {
			facts.MahalanobisSNR, facts.HasMahalanobis = val, true
		}
	}

	if valueStr, ok := metadata[MetadataMaturity]; ok {
		if val, err := strconv.ParseFloat(valueStr, 64); err == nil {
			facts.Maturity, facts.HasMaturity = val, true
		}
	}

	return facts
}
