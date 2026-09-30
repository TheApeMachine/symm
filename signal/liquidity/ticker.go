package liquidity

import (
	"context"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Ticker is the asynchronous touch-liquidity instrument. It holds no state and
no logic of its own: its entire behavior is one nomagique pipeline over the
measurement itself — every stage writes its facts into the measurement where
it computes them, and the workload's register owns the measurement's lifetime.
*/
type Ticker struct {
	*runtime.System
	pipelines sync.Map
	ID        int
}

func NewTicker(ctx context.Context) *Ticker {
	ticker := &Ticker{}

	ticker.System = runtime.NewSystem(ctx, "liquidity:ticker", ticker)
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
		NewGate(),
		NewTouch(),
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

	if len(measurement.Peers) > 0 {
		peer := measurement.FindPeer(func(candidate *data.Measurement[float64]) bool {
			_, hasBid := candidate.LookupMetric("bid")
			_, hasAsk := candidate.LookupMetric("ask")
			_, hasBidQuantity := candidate.LookupMetric("bid_qty")
			_, hasAskQuantity := candidate.LookupMetric("ask_qty")

			return hasBid && hasAsk && hasBidQuantity && hasAskQuantity && candidate.Label != ""
		})

		if peer == nil {
			return nil
		}

		measurement.Pull(peer, "bid", "ask", "bid_qty", "ask_qty")
	}

	if _, hasBid := measurement.LookupMetric("bid"); !hasBid {
		return measurement
	}

	if _, hasAsk := measurement.LookupMetric("ask"); !hasAsk {
		return measurement
	}

	res := data.Read[*data.Measurement[float64]](ticker.pipelineFor(measurement.Label).Next(
		transport.NewOne(unsafe.Pointer(&measurement)).Next(nil),
	))

	if res == nil {
		return measurement
	}

	return res
}
