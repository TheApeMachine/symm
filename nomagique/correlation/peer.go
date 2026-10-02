package correlation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
PeerEnergy extracts focal and peer return energy rates and peer timestamps.
*/
type PeerEnergy struct {
	*core.PrimitiveError
}

func NewPeerEnergy() core.Primitive {
	return &PeerEnergy{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *PeerEnergy) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement[float64])(arriving)

			if len(m.Peers) > 0 {
				var minPeerFrom int64
				var first bool
				for _, p := range m.Peers {
					if !first || p.From.UnixNano() < minPeerFrom {
						minPeerFrom = p.From.UnixNano()
						first = true
					}
				}
				m.WriteMetric("PeerFrom", float64(minPeerFrom)*1e-9)
			}

			if val, ok := m.LookupMetric("return_energy_rate:measured"); ok {
				m.WriteMetric("focal_return_energy_rate", val.Raw)
			}
			if val, ok := m.LookupMetric("peer_return_energy_rate"); ok {
				m.WriteMetric("relative_cohort_return_energy", val.Raw)
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
