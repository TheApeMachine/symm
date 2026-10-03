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
				spread := ask - bid

				if !op.hasFrom {
					op.fromMidpoint = mid
					op.hasFrom = true
				}

				m.SetMetric("response_midpoint:from", data.NewMetric[float64](
					"response_midpoint:from",
					data.UnitPrice,
					data.TimescaleEpoch,
					op.fromMidpoint,
					spread,
				).Write(op.fromMidpoint))
				m.SetMetric("response_midpoint:at", data.NewMetric[float64](
					"response_midpoint:at",
					data.UnitPrice,
					data.TimescaleInstantaneous,
					mid,
					spread,
				).Write(mid))

				logReturn := math.Log(mid / op.fromMidpoint)
				returnScale := math.Max(spread/mid, 1e-6)
				m.SetMetric("midpoint_log_return", data.NewMetric[float64](
					"midpoint_log_return",
					data.UnitLogReturn,
					data.TimescaleRollingWindow,
					0.0,
					returnScale,
				).Write(logReturn))

				elapsed := m.At.Sub(m.From).Seconds()
				if elapsed > 0 {
					returnRate := logReturn / elapsed
					m.SetMetric("midpoint_return_rate", data.NewMetric[float64](
						"midpoint_return_rate",
						data.UnitRate,
						data.TimescalePerSecond,
						0.0,
						math.Max(math.Abs(returnRate), 1e-6),
					).Write(returnRate))

					netNotional := m.GetMetric("net_notional").Raw
					if netNotional != 0 {
						sign := 1.0
						if netNotional < 0 {
							sign = -1.0
						}

						m.SetMetric("flow_aligned_midpoint_return", data.NewMetric[float64](
							"flow_aligned_midpoint_return",
							data.UnitLogReturn,
							data.TimescaleRollingWindow,
							0.0,
							returnScale,
						).Write(sign*logReturn))
						m.SetMetric("midpoint_response_per_net_notional", data.NewMetric[float64](
							"midpoint_response_per_net_notional",
							data.UnitRatio,
							data.TimescaleRollingWindow,
							0.0,
							math.Max(math.Abs(logReturn/netNotional), 1e-9),
						).Write(logReturn/netNotional))
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
