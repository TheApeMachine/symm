package hawkes

import (
	"context"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmhawkes "github.com/theapemachine/symm/nomagique/statistic/hawkes"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Trade is the Hawkes arrival-dynamics instrument. It holds no estimation state
of its own: its entire behavior is one nomagique pipeline over the
measurement itself — every stage writes its facts into the measurement where
it computes them, and the workload's register owns the measurement's
lifetime. The per-symbol arrival paths and fitted models live inside the
pipeline's shared stage registry.
*/
type Trade struct {
	*runtime.System
	pipelines sync.Map
	ID        int
}

/*
NewTrade composes the arrival-dynamics pipeline: the gate classifies the
trade's side, the counts stage admits the arrival into the symbol's
observation window, the excitation stage measures the arrival against the
model fitted before it, and the refit stage folds the arrival into the
history and re-estimates for the next one.
*/
func NewTrade(ctx context.Context) *Trade {

	trade := &Trade{}

	trade.System = runtime.NewSystem(ctx, "hawkes:trade", trade)
	return trade
}

/*
Step supplies public spot trade arrivals to the pipeline. Book mutations and
futures arrivals are different point processes, even when they share a symbol
and carry price and quantity fields.
*/

func (trade *Trade) pipelineFor(symbol string) core.Primitive {
	if existing, ok := trade.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	history := nmhawkes.Paths()

	pipeline := nomagique.NewNumber(
		nmhawkes.NewGate(),
		nmhawkes.NewCounts(history),
		nmhawkes.NewExcitation(history),
		nmhawkes.NewRefit(history),
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

	if len(measurement.Peers) > 0 {
		measurement.Err = nil

		peer := measurement.FindPeer(func(candidate *data.Measurement[float64]) bool {
			_, hasPrice := candidate.LookupMetric("price")
			_, hasQty := candidate.LookupMetric("qty")
			return candidate.Provenance["channel"] == "trade" &&
				hasPrice && hasQty && candidate.Label != "" && candidate.Err == nil
		})

		if peer == nil {
			return nil
		}

		measurement.Reset()
		measurement.Pull(peer, "price", "qty")
	}

	if measurement.Err != nil {
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
