package cvd

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Response measures the quote-midpoint price movement contemporaneous with
the executed aggressive flow.
*/
type Response struct {
	err          error
	hasFrom      bool
	fromMidpoint float64
}

func NewResponse() core.Primitive {
	return &Response{}
}

func (op *Response) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			var bid, ask float64
			for _, bidKey := range []string{"best_bid", "bid", "best_bid_price", "bid_price"} {
				if metric, ok := m.LookupMetric(bidKey); ok && metric.Raw > 0 {
					bid = metric.Raw
					break
				}
			}

			for _, askKey := range []string{"best_ask", "ask", "best_ask_price", "ask_price"} {
				if metric, ok := m.LookupMetric(askKey); ok && metric.Raw > 0 {
					ask = metric.Raw
					break
				}
			}

			if bid > 0 && ask > bid {
				mid := (bid + ask) / 2.0

				if !op.hasFrom {
					op.fromMidpoint = mid
					op.hasFrom = true
				}

				m.WriteMetric("response_midpoint:from", op.fromMidpoint)
				m.WriteMetric("response_midpoint:at", mid)

				logReturn := math.Log(mid / op.fromMidpoint)
				m.WriteMetric("midpoint_log_return", logReturn)

				elapsed := m.At.Sub(m.From).Seconds()
				if elapsed > 0 {
					returnRate := logReturn / elapsed
					m.WriteMetric("midpoint_return_rate", returnRate)

					netNotional := m.GetMetric("net_notional").Raw
					if netNotional != 0 {
						sign := 1.0
						if netNotional < 0 {
							sign = -1.0
						}

						m.WriteMetric("flow_aligned_midpoint_return", sign*logReturn)
						m.WriteMetric("midpoint_response_per_net_notional", logReturn/netNotional)
					}
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Response) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
