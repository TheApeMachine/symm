package derivatives

import (
	"context"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	nmderivatives "github.com/theapemachine/symm/nomagique/derivatives"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Trade is the liquidation-notional accounting instrument. It holds no state
and no logic of its own: its entire behavior is one nomagique pipeline over
the measurement itself — every stage writes its facts into the measurement
where it computes them, and the workload's register owns the measurement's
lifetime.
*/
type Trade struct {
	*runtime.System
	pipelines sync.Map
	ID        int
}

func NewTrade(ctx context.Context) *Trade {
	trade := &Trade{}

	trade.System = runtime.NewSystem(ctx, "derivatives:trade", trade)
	return trade
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/

func (trade *Trade) pipelineFor(symbol string) core.Primitive {
	if existing, ok := trade.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	pipeline := nomagique.NewNumber(
		nmderivatives.NewTradeGate(),
		nmderivatives.NewLiquidation(),
		data.NewFinalizer[float64](),
	)

	actual, _ := trade.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

func (trade *Trade) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if trade.Status() != runtime.READY {
		errnie.Warn(trade.Name() + ": Step called before READY; dropping event")
		return measurement
	}

	if measurement == nil {
		return measurement
	}

	measurement.SetSource("derivatives:trade")

	if len(measurement.Peers) > 0 {
		peer := measurement.FindPeer(func(candidate *data.Measurement[float64]) bool {
			if candidate.Label == "" {
				return false
			}

			_, hasPrice := candidate.LookupMetric("price")
			_, hasQty := candidate.LookupMetric("qty")
			return hasPrice && hasQty
		})

		if peer == nil {
			return nil
		}

		measurement.Pull(peer, "price", "qty")
	}

	res := data.Read[*data.Measurement[float64]](trade.pipelineFor(measurement.Label).Next(
		transport.NewOne(unsafe.Pointer(&measurement)).Next(nil),
	))

	if res == nil {
		return measurement
	}

	return res
}
