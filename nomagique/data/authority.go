package data

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/equation"
)

/*
Authority preserves the source measurement's policy factors: .5 for an
estimated reading with unknown SNR, .1 for defined non-positive SNR. They
are retained compatibility policy, not calibrated probabilities.
*/
type Authority struct {
	*core.PrimitiveError

	authority core.Primitive
	unknown   float64
	zero      float64
	out       float64
}

/*
NewAuthority creates the evidence-authority weighting primitive over quality
readings.
*/
func NewAuthority() *Authority {
	return &Authority{PrimitiveError: core.NewPrimitiveError(), authority: equation.NewEvidenceAuthority(),
		unknown: 0.5,
		zero:    0.1,
	}
}

/*
Next receives *QualityReading payloads and yields a *float64 authority weight
for each.
*/
func (authority *Authority) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			reading := (*QualityReading)(arriving)
			value, err := authority.weight(*reading)

			if err != nil {
				authority.Error(err)
				return
			}

			authority.out = value

			if !yield(unsafe.Pointer(&authority.out)) {
				return
			}
		}
	}
}

/*
weight drives the canonical evidence-authority equation for one reading.
*/
func (authority *Authority) weight(reading QualityReading) (float64, error) {
	var value float64

	for out := range authority.authority.Next(sequence.NewValues(equation.EvidenceAuthorityInput{
		Estimated:  reading.Estimated,
		SNRDefined: reading.SNRDefined,
		SNR:        reading.SNR,
		Maturity:   reading.Maturity,
		Unknown:    authority.unknown,
		Zero:       authority.zero,
	}).Next(nil)) {
		value = *(*float64)(out)
	}

	if err := authority.authority.Error(); err != nil {
		return 0, err
	}

	return value, nil
}
