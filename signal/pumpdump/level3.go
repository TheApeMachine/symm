package pumpdump

import (
	"fmt"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Level3 is the authoritative executable-touch market entity. It owns exactly a
Number pipeline and a projector, both declared in its constructor, plus Step
and Close. It retains no book: each Step derives the touch directly from the
message's own visible bid/ask orders.
*/
type Level3 struct {
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewLevel3 constructs the Level3 entity: one Number pipeline for the executable
touch, and one projector that names the output slots.
*/
func NewLevel3() *Level3 {
	return &Level3{
		number: nomagique.NewNumber[string](nmtypes.Pipe(
			// 0 < bid < ask: a crossed, missing, or non-positive book is rejected here.
			logic.PositiveOrder(symbolBidPrice, symbolAskPrice),
			nmtypes.Wire(
				calculus.Average,
				nmtypes.In(symbolBidPrice, calculus.PortA),
				nmtypes.In(symbolAskPrice, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolMidpoint),
			),
			nmtypes.Wire(
				calculus.Difference,
				nmtypes.In(symbolAskPrice, calculus.PortA),
				nmtypes.In(symbolBidPrice, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolSpread),
			),
			nmtypes.Wire(
				calculus.Quotient,
				nmtypes.In(symbolSpread, calculus.PortA),
				nmtypes.In(symbolMidpoint, calculus.PortB),
				nmtypes.Out(calculus.PortResult, symbolRelativeSpread),
			),
		)),
		projector: data.NewProjector(
			data.Binding{From: symbolBidPrice, Name: "best_bid", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolAskPrice, Name: "best_ask", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolMidpoint, Name: "midpoint", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolSpread, Name: "spread", Unit: data.UnitRate, Timescale: data.TimescaleInstantaneous},
			data.Binding{From: symbolRelativeSpread, Name: "relative_spread", Unit: data.UnitDimensionless, Timescale: data.TimescaleInstantaneous},
		),
	}
}

/*
Step derives the executable touch directly from this message's own visible
bid/ask orders (best bid, best ask) and projects it. A message with no usable
touch on either side yields a rejection, not a panic.
*/
func (level3 *Level3) Step(message kraken.Level3Data) *data.Measurement[float64] {
	if level3 == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("pumpdump: level3 entity missing for %s", message.Symbol)}
	}

	bidPrice := 0.0
	askPrice := 0.0

	for _, order := range message.Bids {
		if order.LimitPrice == nil {
			continue
		}

		if price := order.LimitPrice.Float64(); price > bidPrice {
			bidPrice = price
		}
	}

	for _, order := range message.Asks {
		if order.LimitPrice == nil {
			continue
		}

		if price := order.LimitPrice.Float64(); askPrice == 0 || price < askPrice {
			askPrice = price
		}
	}

	if bidPrice == 0 || askPrice == 0 {
		return &data.Measurement[float64]{Err: fmt.Errorf("pumpdump: book touch missing for %s", message.Symbol)}
	}

	input := nmtypes.Frame{}
	input.Put(symbolBidPrice, bidPrice)
	input.Put(symbolAskPrice, askPrice)

	return level3.projector.Project(
		message.Symbol,
		"pumpdump",
		message.Timestamp,
		message.Timestamp,
		level3.number.Step(message.Symbol, input),
	)
}

func (level3 *Level3) Close() error { return nil }
