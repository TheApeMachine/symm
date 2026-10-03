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
				fromSec := float64(minPeerFrom) * 1e-9
				m.SetMetric("PeerFrom", data.NewMetric[float64](
					"PeerFrom",
					data.UnitSecond,
					data.TimescaleInstantaneous,
					fromSec,
					1.0,
				).Write(fromSec))
			}

			if val, ok := m.LookupMetric("return_energy_rate:measured"); ok {
				m.SetMetric("focal_return_energy_rate", data.NewMetric[float64](
					"focal_return_energy_rate",
					val.Unit,
					val.Timescale,
					val.Center,
					val.Scale,
				).Write(val.Raw))
			}
			if val, ok := m.LookupMetric("peer_return_energy_rate"); ok {
				m.SetMetric("relative_cohort_return_energy", data.NewMetric[float64](
					"relative_cohort_return_energy",
					val.Unit,
					val.Timescale,
					val.Center,
					val.Scale,
				).Write(val.Raw))
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
