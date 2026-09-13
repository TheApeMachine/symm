package derivatives

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
Gate classifies a ticker arrival: it reads the derivative, reference, and
spot prices and the open interest the feed wrote, and rejects anything the
basis arithmetic cannot score. Anything invalid fails the measurement here
and never reaches the derivative state. The prices are rewritten on every
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

			last, traded := m.Metrics["last"]
			index, referenced := m.Metrics["index_price"]
			mark, spotted := m.Metrics["mark_price"]
			oi, interested := m.Metrics["open_interest"]

			if !traded || !referenced || !spotted || !interested {
				m.Err = fmt.Errorf(
					"%w: derivatives: ticker requires a last, index, mark, and open interest",
					core.ErrDomain,
				)

				if !yield(arriving) {
					return
				}

				continue
			}

			if holds := drive[float64, bool](op.finite, &last.Raw); !holds || index.Raw <= 0 || mark.Raw <= 0 || last.Raw < 0 || oi.Raw < 0 {
				m.Err = fmt.Errorf(
					"%w: derivatives: non-negative finite prices and non-negative open interest required",
					core.ErrDomain,
				)

				if !yield(arriving) {
					return
				}

				continue
			}

			m.Metrics["last"] = last.Write(last.Raw)
			m.Metrics["index_price"] = index.Write(index.Raw)
			m.Metrics["mark_price"] = mark.Write(mark.Raw)
			m.Metrics["open_interest"] = oi.Write(oi.Raw)

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

/*
TradeGate classifies a futures trade arrival: it reads the executed price and
quantity the feed wrote and rejects anything the liquidation accounting
cannot score. The trade's categorical side and type travel in provenance.
*/
type TradeGate struct {
	err    error
	finite core.Primitive
}

func NewTradeGate() core.Primitive {
	return &TradeGate{finite: logic.NewFinite()}
}

func (op *TradeGate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			price, priced := m.Metrics["price"]
			quantity, quantified := m.Metrics["qty"]

			if !priced || !quantified {
				m.Err = fmt.Errorf("%w: derivatives: trade requires a price and a quantity", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			notional := price.Raw * quantity.Raw

			if holds := drive[float64, bool](op.finite, &price.Raw); !holds {
				m.Err = fmt.Errorf("%w: derivatives: finite price required", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			if holds := drive[float64, bool](op.finite, &quantity.Raw); !holds {
				m.Err = fmt.Errorf("%w: derivatives: finite quantity required", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			if holds := drive[float64, bool](op.finite, &notional); !holds {
				m.Err = fmt.Errorf("%w: derivatives: finite notional required", core.ErrDomain)

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

func (op *TradeGate) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

