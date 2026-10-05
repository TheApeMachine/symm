package data

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Adapter wraps a primitive to translate between the domain's Measurement and the
primitive's specialized input/output types. It hijacks the Next method, intercepts
the arriving Measurement, transforms its data into the input shape expected by
the wrapped primitive, and then transforms the yielded result back into the Measurement.
*/
type Adapter[In, Out any] struct {
	err   error
	op    core.Primitive
	read  func(m *Measurement[float64]) In
	write func(m *Measurement[float64], out Out)
}

func NewAdapter[In, Out any](
	op core.Primitive,
	read func(m *Measurement[float64]) In,
	write func(m *Measurement[float64], out Out),
) core.Primitive {
	return &Adapter[In, Out]{
		op:    op,
		read:  read,
		write: write,
	}
}

func (wrapper *Adapter[In, Out]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**Measurement[float64])(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}
				continue
			}

			m.EnsureMetadata()

			// 1. Manipulate incoming: extract the specialized input shape
			inputShape := wrapper.read(m)

			// 2. Call the wrapped primitive internally
			for outPtr := range wrapper.op.Next(transport.NewOne(unsafe.Pointer(&inputShape)).Next(nil)) {
				// 3. Manipulate outgoing: write the specialized output shape back to the measurement
				outShape := *(*Out)(outPtr)
				wrapper.write(m, outShape)
			}

			// Yield the enriched measurement
			if !yield(arriving) {
				return
			}
		}
	}
}

func (wrapper *Adapter[In, Out]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			wrapper.err = errors.Join(wrapper.err, err)
		}
	}
	if wrapper.op != nil {
		if err := wrapper.op.Error(); err != nil {
			wrapper.err = errors.Join(wrapper.err, err)
		}
	}
	return wrapper.err
}
