package pumpdump

import (
	"time"

	book "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Level3 is the authoritative executable-touch market entity. It reads the shared
book and owns exactly a Number pipeline and a projector, both declared in its
constructor, plus Step and Close.
*/
type Level3 struct {
	workspace *runtime.Workspace
	number    *nomagique.Number[string]
	projector *data.Projector
}

/*
NewLevel3 constructs the Level3 entity: one Number pipeline for the executable
touch read from the shared book, and one projector that names the output slots.
*/
func NewLevel3(workspace *runtime.Workspace) *Level3 {
	return &Level3{
		workspace: workspace,
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
Step reads the shared book for one symbol and projects the executable touch.
The shared book is type-asserted to *book.Book; a missing book or a missing
touch field panics rather than being silently swallowed.
*/
func (level3 *Level3) Step(symbol string, at time.Time) *data.Measurement[float64] {
	shared, _ := level3.workspace.Shared("book", symbol)
	resident := shared.(*book.Book)

	input := nmtypes.Frame{}
	input.Put(symbolBidPrice, resident.BestBid().Price.Float64())
	input.Put(symbolAskPrice, resident.BestAsk().Price.Float64())

	return level3.projector.Project(
		symbol,
		"pumpdump",
		at,
		at,
		level3.number.Step(symbol, input),
	)
}

func (level3 *Level3) Close() error { return nil }
