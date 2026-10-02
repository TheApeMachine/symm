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
	arena     *data.ArenaOwner
	pipelines sync.Map
	ID        int
}

func NewTicker(ctx context.Context, arena *data.ArenaOwner) *Ticker {
	ticker := &Ticker{
		arena: arena,
	}

	ticker.System = runtime.NewSystem(ctx, "leadlag:ticker", ticker)
	return ticker
}

func (ticker *Ticker) Source() string {
	return "leadlag:ticker"
}

func (ticker *Ticker) Arena() *data.ArenaOwner {
	return ticker.arena
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

func (ticker *Ticker) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if ticker.Status() != runtime.READY {
		errnie.Warn(ticker.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	price := quotedPrice(prior)
	if price <= 0 {
		return nil
	}

	out := ticker.arena.NewMeasurement(ticker.Source())
	out.Label = prior.Label
	out.SeqIdx = prior.SeqIdx
	out.At = prior.At
	out.From = prior.From
	out.Peers = []*data.Measurement[float64]{prior}

	out.WriteMetric("last", price)
	out.WriteMetric("last_price", price)

	if channel, hasCh := prior.GetProvenance("channel"); hasCh {
		out.SetProvenance("channel", channel)
	}

	res := data.Read[*data.Measurement[float64]](ticker.pipelineFor(out.Label).Next(
		transport.NewOne(unsafe.Pointer(&out)).Next(nil),
	))

	if res == nil {
		return out
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
