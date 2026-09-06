package vector

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewFinite asks whether every scalar in the incoming run is finite.
func NewFinite() core.Primitive {
	return transport.NewPipe(transport.NewMap(logic.NewFinite()), logic.NewAnd(transport.NewIO(core.From(true))))
}
