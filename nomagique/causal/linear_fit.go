package causal

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewLinearFit composes the supplied table's affine design into ordinary least
// squares. Input fields are rows, target (float64 index), features ([]float64
// indices). The caller declares the exact feature list; no automatic removal,
// addition, ordering or deduplication of predictors occurs here.
func NewLinearFit(tolerance core.Primitive) core.Primitive {
	context := store.NewRetained(nil)
	return transport.NewPipe(
		context,
		store.NewRecord(
			transport.NewPipe(
				store.NewGet("rows"),
				transport.NewSpread[[]float64](),
				transport.NewMap(
					equation.NewDesign(
						transport.NewApply(transport.NewPipe(store.NewGet("features"), transport.NewSpread[float64]()), context)),
				),
				transport.NewCollect[[]float64](),
				store.NewKey("x"),
			),
			transport.NewPipe(
				store.NewGet("rows"),
				transport.NewSpread[[]float64](),
				transport.NewMap(collection.NewAt[float64](
					transport.NewApply(store.NewGet("target"), context))),
				transport.NewCollect[float64](),
				store.NewKey("y"),
			),
		),
		algo.NewOLS(tolerance),
	)
}
