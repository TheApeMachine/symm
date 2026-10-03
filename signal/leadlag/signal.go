package leadlag

import (
	"context"
	"math"
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
type Signal struct {
	*runtime.System
	arena     *data.ArenaOwner
	pipelines sync.Map
	ID        int
}

func NewSignal(ctx context.Context, arena *data.ArenaOwner) *Signal {
	signal := &Signal{
		arena: arena,
	}

	signal.System = runtime.NewSystem(ctx, "leadlag", signal)
	return signal
}

func (signal *Signal) Arena() *data.ArenaOwner {
	return signal.arena
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/

func (signal *Signal) pipelineFor(symbol string) core.Primitive {
	if existing, ok := signal.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	pipeline := nomagique.NewNumber(
		nmleadlag.NewGate(),
		nmleadlag.NewCross(algo.NewHayashiYoshida()),
		nmleadlag.NewPeerEnergy(),
		data.NewRecurrence(
			"best_lag_seconds",
			"best_lag_correlation",
			"correlation_gain_zscore",
		),
		data.NewFinalizer[float64](),
	)

	actual, _ := signal.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

func (signal *Signal) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	price := quotedPrice(prior)
	if price <= 0 {
		return nil
	}

	out := signal.arena.NewMeasurement(signal.Name())
	out.Epoch = prior.Epoch
	out.Tick = prior.Tick
	out.Label = prior.Label
	out.SeqIdx = prior.SeqIdx
	out.At = prior.At
	out.From = prior.From
	out.Peers = []*data.Measurement[float64]{prior}

	midpoint := price
	spread := 0.0

	bid := prior.GetMetric("best_bid").Raw
	if bid == 0 {
		bid = prior.GetMetric("bid").Raw
	}
	ask := prior.GetMetric("best_ask").Raw
	if ask == 0 {
		ask = prior.GetMetric("ask").Raw
	}

	if bid > 0 && ask > bid {
		midpoint = (bid + ask) / 2.0
		spread = ask - bid
	} else if midMetric, ok := prior.LookupMetric("midpoint"); ok && midMetric.Raw > 0 {
		midpoint = midMetric.Raw
		if spreadMetric, ok := prior.LookupMetric("spread"); ok && spreadMetric.Raw > 0 {
			spread = spreadMetric.Raw
		}
	}

	if spread <= 0 {
		spread = math.Max(price*0.0001, 1e-6)
	}

	out.SetMetric("last", data.NewMetric[float64](
		"last",
		data.UnitPrice,
		data.TimescaleTick,
		midpoint,
		spread,
	).Write(price))

	out.SetMetric("last_price", data.NewMetric[float64](
		"last_price",
		data.UnitPrice,
		data.TimescaleTick,
		midpoint,
		spread,
	).Write(price))

	if channel, hasCh := prior.GetProvenance("channel"); hasCh {
		out.SetProvenance("channel", channel)
	}

	res := data.Read[*data.Measurement[float64]](signal.pipelineFor(out.Label).Next(
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
