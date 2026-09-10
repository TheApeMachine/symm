package data

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
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
	core.Base[QualityFacts, QualityReading]
	finite *logic.Finite[float64]
}

func NewQuality() *Quality {
	return &Quality{finite: logic.NewFinite[float64]()}
}

func (op *Quality) Next(
	in iter.Seq[core.Primitive[QualityFacts, QualityFacts]],
) iter.Seq[core.Primitive[QualityReading, QualityReading]] {
	return func(yield func(core.Primitive[QualityReading, QualityReading]) bool) {
		for arriving := range in {
			reading, err := op.Derive(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(reading)) {
				return
			}
		}
	}
}

func (op *Quality) Derive(facts QualityFacts) (QualityReading, error) {
	if err := op.finiteField(facts.HasSupport, facts.Support); err != nil {
		return QualityReading{}, err
	}

	if err := op.finiteField(facts.HasDivergence, facts.Divergence); err != nil {
		return QualityReading{}, err
	}

	if err := op.finiteField(facts.HasNoise, facts.NoiseVariance); err != nil {
		return QualityReading{}, err
	}

	if err := op.finiteField(facts.HasMahalanobis, facts.MahalanobisSNR); err != nil {
		return QualityReading{}, err
	}

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

		return reading, nil
	}

	if facts.HasMaturity {
		reading.Maturity = facts.Maturity
	}

	return reading, nil
}

func (op *Quality) finiteField(present bool, value float64) error {
	if !present {
		return nil
	}

	defined, err := transport.Evaluate(op.finite, transport.Values(value))

	if err != nil {
		return err
	}

	if !defined {
		return core.ErrDomain
	}

	return nil
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
