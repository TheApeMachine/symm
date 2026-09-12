package data

import (
	"errors"
	"iter"
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

			if facts.HasDivergence && facts.HasNoise && facts.NoiseVariance > 0 {
				reading.SNR = facts.Divergence * facts.Divergence / facts.NoiseVariance
				reading.SNRDefined = true
			}

			if facts.HasSupport {
				reading.Maturity = 0

				if facts.Support > 1 {
					reading.Maturity = 1 - 1/facts.Support

					if facts.HasMahalanobis && facts.MahalanobisSNR >= 0 {
						reading.SNR = facts.MahalanobisSNR
						reading.SNRDefined = true
					}
				}
			} else if facts.HasMaturity {
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

func factsFromMetadata(metadata map[string]float64) QualityFacts {
	facts := QualityFacts{}

	if metadata == nil {
		return facts
	}

	if value, ok := metadata[MetadataSupport]; ok {
		facts.Support, facts.HasSupport = value, true
	}

	if value, ok := metadata[MetadataDivergence]; ok {
		facts.Divergence, facts.HasDivergence = value, true
	}

	if value, ok := metadata[MetadataNoiseVariance]; ok {
		facts.NoiseVariance, facts.HasNoise = value, true
	}

	if value, ok := metadata[MetadataMahalanobisSNR]; ok {
		facts.MahalanobisSNR, facts.HasMahalanobis = value, true
	}

	if value, ok := metadata[MetadataMaturity]; ok {
		facts.Maturity, facts.HasMaturity = value, true
	}

	return facts
}
