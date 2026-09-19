package data

import (
	"strconv"

	"github.com/theapemachine/symm/nomagique/types"
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
NewQuality owns quality derivation. Support>1 is required before Mahalanobis
overrides scalar SNR.
No structs, pure Value closure.
*/
type Quality types.Value[QualityFacts, QualityReading]
func NewQuality() Quality {
	return func(facts QualityFacts) QualityReading {
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

		return reading
	}
}

func factsFromMetadata(metadata map[string]string) QualityFacts {
	facts := QualityFacts{}

	if metadata == nil {
		return facts
	}

	if val, ok := metadata[MetadataSupport]; ok {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			facts.Support = parsed
			facts.HasSupport = true
		}
	}

	if val, ok := metadata[MetadataDivergence]; ok {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			facts.Divergence = parsed
			facts.HasDivergence = true
		}
	}

	if val, ok := metadata[MetadataNoiseVariance]; ok {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			facts.NoiseVariance = parsed
			facts.HasNoise = true
		}
	}

	if val, ok := metadata[MetadataMahalanobisSNR]; ok {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			facts.MahalanobisSNR = parsed
			facts.HasMahalanobis = true
		}
	}

	if val, ok := metadata[MetadataMaturity]; ok {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			facts.Maturity = parsed
			facts.HasMaturity = true
		}
	}

	return facts
}
