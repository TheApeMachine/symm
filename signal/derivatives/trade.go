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

func (trade *Trade) Step(input *runtime.StageInput, output *data.Measurement[float64]) *data.Measurement[float64] {
	if trade.Status() != runtime.READY {
		errnie.Warn(trade.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if input == nil || input.Ingress() == nil || input.Symbol() == "" {
		return nil
	}

	priceMetric, hasPrice := input.IngressMetric("price")
	qtyMetric, hasQty := input.IngressMetric("qty")

	if !hasPrice || !hasQty {
		return nil
	}

	output.SetMetric("price", priceMetric)
	output.SetMetric("qty", qtyMetric)

	if side, hasSide := input.IngressProvenance("side"); hasSide {
		output.SetProvenance("side", side)
	}

	if channel, hasCh := input.IngressProvenance("channel"); hasCh {
		output.SetProvenance("channel", channel)
	}

	data.StampInterval(output, input.At(), input.From())

	res := data.Read[*data.Measurement[float64]](trade.pipelineFor(output.Label).Next(
		transport.NewOne(unsafe.Pointer(&output)).Next(nil),
	))

	if res == nil {
		return output
	}

	return res
}
