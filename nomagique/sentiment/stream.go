package sentiment

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
Gate classifies the arrival: it reads the last price the feed wrote, consumes
that metric, and stamps the measurement's support baseline. Anything invalid
fails the measurement here and never reaches the path. The last price and the
metadata baseline are rewritten on every arrival, so the measurement carries
this arrival's facts, never the prior one's.
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

			metric, traded := m.Metrics["last"]

			if !traded {
				m.Err = fmt.Errorf("%w: sentiment: ticker requires a last price", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			last := metric.Raw
			m.Metadata = map[string]float64{data.MetadataSupport: 0}

			if holds := drive[float64, bool](op.finite, &last); !holds || last < 0 {
				m.Err = fmt.Errorf("%w: sentiment: finite non-negative last price required", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			m.Metrics["last"] = metric.Write(last)

			if last == 0 {
				m.Provenance = map[string]string{"last_trade_price_state": "unobserved"}
			}

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
