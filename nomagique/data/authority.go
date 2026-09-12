package data

import (
	"errors"
	"iter"
	"unsafe"

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
	err       error
	authority core.Primitive
	unknown   float64
	zero      float64
	out       float64
}

/*
NewAuthority creates the evidence-authority weighting primitive over quality
readings.
*/
func NewAuthority() core.Primitive {
	return &Authority{
		authority: equation.NewEvidenceAuthority(),
		unknown:   0.5,
		zero:      0.1,
	}
}

/*
Next receives *QualityReading payloads and yields a *float64 authority weight
for each.
*/
func (op *Authority) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			reading := (*QualityReading)(arriving)
			value, err := op.weight(*reading)

			if err != nil {
				op.Error(err)
				return
			}

			op.out = value

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Authority) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
weight drives the canonical evidence-authority equation for one reading.
*/
func (op *Authority) weight(reading QualityReading) (float64, error) {
	var value float64

	for out := range op.authority.Next(transport.NewValues(equation.EvidenceAuthorityInput{
		Estimated:  reading.Estimated,
		SNRDefined: reading.SNRDefined,
		SNR:        reading.SNR,
		Maturity:   reading.Maturity,
		Unknown:    op.unknown,
		Zero:       op.zero,
	}).Next(nil)) {
		value = *(*float64)(out)
	}

	if err := op.authority.Error(); err != nil {
		return 0, err
	}

	return value, nil
}
