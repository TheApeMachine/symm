package data

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Authority preserves the source measurement's policy factors: .5 for an
estimated reading with unknown SNR, .1 for defined non-positive SNR. They
are retained compatibility policy, not calibrated probabilities.
*/
type Authority struct {
	core.Base[QualityReading, float64]
	evidence *equation.EvidenceAuthority
	unknown  float64
	zero     float64
}

func NewAuthority() *Authority {
	return &Authority{
		evidence: equation.NewEvidenceAuthority(),
		unknown:  0.5,
		zero:     0.1,
	}
}

func (op *Authority) Next(
	in iter.Seq[core.Primitive[QualityReading, QualityReading]],
) iter.Seq[core.Primitive[float64, float64]] {
	return func(yield func(core.Primitive[float64, float64]) bool) {
		for arriving := range in {
			value, err := op.Weight(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(value)) {
				return
			}
		}
	}
}

func (op *Authority) Weight(reading QualityReading) (float64, error) {
	return transport.Evaluate(op.evidence, transport.Values(equation.EvidenceAuthorityInput{
		Estimated:  reading.Estimated,
		SNRDefined: reading.SNRDefined,
		SNR:        reading.SNR,
		Maturity:   reading.Maturity,
		Unknown:    op.unknown,
		Zero:       op.zero,
	}))
}
