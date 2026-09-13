package cvd

import (
	"errors"
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
drive pushes one payload pointer through one primitive and returns the
answer the primitive yielded.
*/
func drive[From, To any](op core.Primitive, payload *From) To {
	var answer To

	for out := range op.Next(transport.NewOne(unsafe.Pointer(payload)).Next(nil)) {
		answer = *(*To)(out)
	}

	return answer
}

/*
Gate classifies the arrival: it reads the executed price, quantity, and
aggressor side the feed wrote, and rejects anything the executed-flow
arithmetic cannot score. Anything invalid fails the measurement here and
never reaches the flow. The price and quantity are rewritten on every
arrival, so the measurement carries this arrival's facts, never the prior
one's.
*/
type Gate struct {
	err    error
	finite core.Primitive
}

func NewGate() core.Primitive {
	return &Gate{finite: logic.NewFinite()}
}

func (op *Gate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			price, priced := m.Metrics["price"]
			quantity, quantified := m.Metrics["qty"]

			if !priced || !quantified {
				m.Err = fmt.Errorf("%w: cvd: trade requires a price and a quantity", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			notional := price.Raw * quantity.Raw

			if holds := drive[float64, bool](op.finite, &price.Raw); !holds {
				m.Err = fmt.Errorf("%w: cvd: finite price required", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			if holds := drive[float64, bool](op.finite, &quantity.Raw); !holds {
				m.Err = fmt.Errorf("%w: cvd: finite quantity required", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			if holds := drive[float64, bool](op.finite, &notional); !holds {
				m.Err = fmt.Errorf("%w: cvd: finite notional required", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			side := m.Provenance["side"]

			if price.Raw <= 0 || quantity.Raw <= 0 || (side != "buy" && side != "sell") {
				m.Err = fmt.Errorf("%w: cvd: positive price and quantity and a known aggressor side required", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			m.Metrics["price"] = price.Write(price.Raw)
			m.Metrics["qty"] = quantity.Write(quantity.Raw)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Gate) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
