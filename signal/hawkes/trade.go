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

func (trade *Trade) Step(input *runtime.StageInput, output *data.Measurement[float64]) *data.Measurement[float64] {
	if trade.Status() != runtime.READY {
		errnie.Warn(trade.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if input == nil || input.Ingress() == nil {
		return nil
	}

	channel, _ := input.IngressProvenance("channel")
	if channel != "trade" {
		return nil
	}

	priceMetric, hasPrice := input.IngressMetric("price")
	qtyMetric, hasQty := input.IngressMetric("qty")

	if !hasPrice || !hasQty || input.Symbol() == "" {
		return nil
	}

	output.SetMetric("price", priceMetric)
	output.SetMetric("qty", qtyMetric)

	if side, hasSide := input.IngressProvenance("side"); hasSide {
		output.SetProvenance("side", side)
	}

	output.SetProvenance("channel", channel)
	data.StampInterval(output, input.At(), input.From())

	res := data.Read[*data.Measurement[float64]](trade.pipelineFor(output.Label).Next(
		transport.NewOne(unsafe.Pointer(&output)).Next(nil),
	))

	if res == nil {
		return output
	}

	return res
}
