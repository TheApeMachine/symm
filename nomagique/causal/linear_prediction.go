package causal

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/vector"
)

// NewLinearPrediction evaluates one row against a configured-by-record affine
// fit. Input fields: fit (OLS output record), features, row. It does not train.
func NewLinearPrediction() core.Primitive {
	context := store.NewRetained(nil)
	return transport.NewPipe(
		context,
		vector.NewDot(
			transport.NewPipe(store.NewGet("fit"), store.NewGet("coefficients"), transport.NewSpread[float64]()),
			transport.NewPipe(
				store.NewGet("row"),
				equation.NewDesign(transport.NewApply(
					transport.NewPipe(store.NewGet("features"), transport.NewSpread[float64]()), context)),
				transport.NewSpread[float64](),
			),
		),
	)
}
