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
	arena     *data.ArenaOwner
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
func NewTrade(ctx context.Context, arena *data.ArenaOwner) *Trade {
	trade := &Trade{
		arena: arena,
	}

	trade.System = runtime.NewSystem(ctx, "hawkes:trade", trade)
	return trade
}

func (trade *Trade) Source() string {
	return "hawkes:trade"
}

func (trade *Trade) Arena() *data.ArenaOwner {
	return trade.arena
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

func (trade *Trade) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if trade.Status() != runtime.READY {
		errnie.Warn(trade.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	channel, _ := prior.GetProvenance("channel")
	if channel != "trade" {
		return nil
	}

	_, hasPrice := prior.LookupMetric("price")
	_, hasQty := prior.LookupMetric("qty")

	if !hasPrice || !hasQty {
		return nil
	}

	out := trade.arena.NewMeasurement(trade.Source())
	out.Label = prior.Label
	out.SeqIdx = prior.SeqIdx
	out.At = prior.At
	out.From = prior.From
	out.Peers = []*data.Measurement[float64]{prior}

	if side, hasSide := prior.GetProvenance("side"); hasSide {
		out.SetProvenance("side", side)
	}

	out.SetProvenance("channel", channel)

	res := data.Read[*data.Measurement[float64]](trade.pipelineFor(out.Label).Next(
		transport.NewOne(unsafe.Pointer(&out)).Next(nil),
	))

	if res == nil {
		return out
	}

	return res
}
