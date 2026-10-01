package cvd

import (
	"context"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	nmcvd "github.com/theapemachine/symm/nomagique/cvd"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Trade is the CVD executed-flow measuring instrument. It holds no state and no
logic of its own: its entire behavior is composed nomagique pipelines per symbol
over the measurements — every stage writes its facts into the measurement where
it computes them, and the workload's register owns the measurement's lifetime.
*/
type Trade struct {
	*runtime.System
	pipelines sync.Map
	ID        int
}

func NewTrade(ctx context.Context) *Trade {
	trade := &Trade{}
	trade.System = runtime.NewSystem(ctx, "cvd:trade", trade)
	return trade
}

func (trade *Trade) pipelineFor(symbol string) core.Primitive {
	if existing, ok := trade.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	pipeline := nomagique.NewNumber(
		nmcvd.NewGate(),
		nmcvd.NewQuantity(),
		nmcvd.NewNotional(),
		nmcvd.NewRates(),
		data.NewFinalizer[float64](),
	)

	actual, _ := trade.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (trade *Trade) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if trade.Status() != runtime.READY {
		errnie.Warn(trade.Name() + ": Step called before READY; dropping event")
		return measurement
	}

	if measurement == nil {
		return measurement
	}

	measurement.Source = "cvd:trade"

	if len(measurement.Peers) > 0 {
		peer := measurement.FindPeer(func(candidate *data.Measurement[float64]) bool {
			_, hasPrice := candidate.LookupMetric("price")
			_, hasQty := candidate.LookupMetric("qty")
			return hasPrice && hasQty && candidate.Label != ""
		})

		if peer == nil {
			return nil
		}

		measurement.Pull(peer, "price", "qty")
	}

	if measurement.Label == "" {
		return measurement
	}

	if _, hasPrice := measurement.LookupMetric("price"); !hasPrice {
		return measurement
	}

	if _, hasQty := measurement.LookupMetric("qty"); !hasQty {
		return measurement
	}

	res := data.Read[*data.Measurement[float64]](trade.pipelineFor(measurement.Label).Next(
		transport.NewOne(unsafe.Pointer(&measurement)).Next(nil),
	))

	if res == nil {
		return measurement
	}

	return res
}
