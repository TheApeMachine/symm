package adaptive

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
Envelope replaces a value with the inclusive moment interval when the
estimator has dispersion.
*/
type Envelope struct {
	*core.PrimitiveError

	moments     core.Primitive
	coefficient core.Primitive
	out         float64
}

func NewEnvelope(
	moments core.Primitive,
	coefficient core.Primitive,
) *Envelope {
	return &Envelope{PrimitiveError: core.NewPrimitiveError(), moments: moments,
		coefficient: coefficient,
	}
}

func (envelope *Envelope) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		defer func() {

			if envelope.moments != nil {
				if err := envelope.moments.Error(); err != nil {
					envelope.Error(err)
				}
			}

			if envelope.coefficient != nil {
				if err := envelope.coefficient.Error(); err != nil {
					envelope.Error(err)
				}
			}
		}()
		for readingPtr := range envelope.moments.Next(in) {
			current := *(*statistic.MomentReading)(readingPtr)
			value := current.Value

			if current.Count > 1 && current.Dispersion > 0 {
				coeffValEval := envelope.coefficient
				var coeffVal float64

				for out := range coeffValEval.Next(sequence.NewValues(current.Count).Next(nil)) {
					coeffVal = *(*float64)(out)
				}

				err := coeffValEval.Error()

				if err != nil {
					envelope.Error(err)
					return
				}

				margin := current.Dispersion * coeffVal
				lower := current.Mean - margin
				upper := current.Mean + margin

				if value < lower {
					value = lower
				} else if value > upper {
					value = upper
				}
			}

			envelope.out = value

			if !yield(unsafe.Pointer(&envelope.out)) {
				return
			}
		}
	}
}
