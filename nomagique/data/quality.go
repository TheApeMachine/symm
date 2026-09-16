package data

import (
	"iter"
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
	*core.PrimitiveError

	out QualityReading
}

func NewQuality() *Quality {
	return &Quality{PrimitiveError: core.NewPrimitiveError()}
}

func (quality *Quality) Next(
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
			}

			if !facts.HasSupport && facts.HasMaturity {
				reading.Maturity = facts.Maturity
			}

			quality.out = reading

			if !yield(unsafe.Pointer(&quality.out)) {
				return
			}
		}
	}
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
