package leadlag

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/logic"
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
	finite core.Primitive
}

func NewExtractPrice() *ExtractPrice {
	return &ExtractPrice{
		PrimitiveError: core.NewPrimitiveError(),
		finite:         logic.NewFinite(),
	}
}

func (extractPrice *ExtractPrice) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			price, found := extractPrice.resolve(m)

			if !found {
				continue
			}

			valid := false

			for out := range extractPrice.finite.Next(sequence.NewOne(unsafe.Pointer(&price)).Next(nil)) {
				valid = *(*bool)(out)
			}

			if !valid || price < 0 {
				continue
			}

			m.Metrics["last_price"] = m.Metrics["last_price"].Write(price)

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
func (extractPrice *ExtractPrice) resolve(m *data.Measurement[float64]) (float64, bool) {
	if len(m.Peers) > 0 {
		for _, peer := range m.Peers {
			if peer == nil || peer.Label == "" {
				continue
			}

			if metric, ok := peer.Metrics["last_price"]; ok && metric.Raw > 0 {
				m.Label = peer.Label
				m.At = peer.At
				return metric.Raw, true
			}

			if metric, ok := peer.Metrics["last"]; ok && metric.Raw > 0 {
				m.Label = peer.Label
				m.At = peer.At
				return metric.Raw, true
			}

			if metric, ok := peer.Metrics["price"]; ok && metric.Raw > 0 {
				m.Label = peer.Label
				m.At = peer.At
				return metric.Raw, true
			}
		}

		return 0, false
	}

	metric, ok := m.Metrics["last"]

	if !ok || metric.Raw <= 0 {
		return 0, false
	}

	return metric.Raw, true
}
