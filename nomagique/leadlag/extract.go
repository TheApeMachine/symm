package leadlag

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
ExtractPrice reads the last trade price from the arriving measurement,
validates it is finite and non-negative, stamps it as last_price, and
yields the measurement. Invalid arrivals are silently dropped.

When the measurement carries Peers, the first peer with a positive last
price is used instead, so the lead-lag pipeline always operates on the
cross-pair's focal price.
*/
type ExtractPrice struct {
	*core.PrimitiveError
}

func NewExtractPrice() core.Primitive {
	return &ExtractPrice{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *ExtractPrice) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement)(arriving)

			metric, found := op.resolve(m)

			if !found || metric.Raw < 0 {
				continue
			}

			center := metric.Center
			scale := metric.Scale
			if scale == 0 {
				center = metric.Raw
				scale = math.Max(metric.Raw*0.001, 1.0)
			}

			m.SetMetric("last_price", data.NewMetric(
				"last_price",
				data.UnitPrice,
				data.TimescaleInstantaneous,
				center,
				scale,
			).Write(metric.Raw))

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
resolve finds the best available last trade price: from a peer if peers
exist, otherwise from the measurement's own "last" metric.
*/
func (op *ExtractPrice) resolve(m *data.Measurement) (data.Metric, bool) {
	if len(m.Peers) > 0 {
		for _, peer := range m.Peers {
			if peer == nil || peer.Label == "" {
				continue
			}

			if metric, ok := peer.LookupMetric("last_price"); ok && metric.Raw > 0 {
				m.Label = peer.Label
				m.At = peer.At
				return metric, true
			}

			if metric, ok := peer.LookupMetric("last"); ok && metric.Raw > 0 {
				m.Label = peer.Label
				m.At = peer.At
				return metric, true
			}

			if metric, ok := peer.LookupMetric("price"); ok && metric.Raw > 0 {
				m.Label = peer.Label
				m.At = peer.At
				return metric, true
			}
		}

		return data.Metric{}, false
	}

	metric, ok := m.LookupMetric("last")

	if !ok || metric.Raw <= 0 {
		return data.Metric{}, false
	}

	return metric, true
}
