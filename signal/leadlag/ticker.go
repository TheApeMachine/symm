package leadlag

import (
	"context"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	nmleadlag "github.com/theapemachine/symm/nomagique/leadlag"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Ticker is the asynchronous price-path lead-lag instrument. It holds no state
and no logic of its own: its entire behavior is one nomagique pipeline over
the measurement itself — every stage writes its facts into the measurement
where it computes them, and the workload's register owns the measurement's
lifetime.
*/
type Ticker struct {
	*runtime.System
	pipelines sync.Map
	ID        int
}

func NewTicker(ctx context.Context) *Ticker {
	ticker := &Ticker{}

	ticker.System = runtime.NewSystem(ctx, "leadlag:ticker", ticker)
	return ticker
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/

func (ticker *Ticker) pipelineFor(symbol string) core.Primitive {
	if existing, ok := ticker.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	pipeline := nomagique.NewNumber(
		nmleadlag.NewGate(),
		nmleadlag.NewCross(algo.NewHayashiYoshida()),
		data.NewAdapter(
			transport.NewPass(),
			func(m *data.Measurement[float64]) *data.Measurement[float64] {
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
				
				return m
			},
			func(m *data.Measurement[float64], out *data.Measurement[float64]) {},
		),
		data.NewFinalizer[float64](),
	)

	actual, _ := ticker.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

func (ticker *Ticker) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if ticker.Status() != runtime.READY {
		errnie.Warn(ticker.Name() + ": Step called before READY; dropping event")
		return measurement
	}

	if measurement == nil {
		return measurement
	}

	measurement.Source = "leadlag:ticker"

	if len(measurement.Peers) > 0 {
		peer := measurement.FindPeer(func(candidate *data.Measurement[float64]) bool {
			if candidate.Label == "" {
				return false
			}

			return quotedPrice(candidate) > 0
		})

		if peer == nil {
			return nil
		}

		price := quotedPrice(peer)
		measurement.Pull(peer)
		measurement.WriteMetric("last", price)
		measurement.WriteMetric("last_price", price)
	}

	res := data.Read[*data.Measurement[float64]](ticker.pipelineFor(measurement.Label).Next(
		transport.NewOne(unsafe.Pointer(&measurement)).Next(nil),
	))

	if res == nil {
		return measurement
	}

	return res
}

func quotedPrice(measurement *data.Measurement[float64]) float64 {
	for _, key := range []string{"last_price", "last", "price"} {
		if metric, ok := measurement.LookupMetric(key); ok && metric.Raw > 0 {
			return metric.Raw
		}
	}

	return 0
}
