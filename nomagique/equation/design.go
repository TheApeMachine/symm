package equation

import (
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewDesign selects configured feature positions and prepends the affine
// intercept. Its output is a design vector; it does not fit or predict.
func NewDesign(features core.Primitive) core.Primitive {
	return transport.NewPipe(
		collection.NewGather[float64](features),
		transport.NewFan(transport.NewPipe(), transport.NewIO(store.NewConstant(core.From(1.0)), transport.NewSpread[float64]())),
		transport.NewCollect[float64](),
	)
}
