package logic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewFinite is not-NaN and not-infinite, assembled from existing predicates.
func NewFinite() core.Primitive {
	return transport.NewPipe(
		transport.NewFan(transport.NewPipe(), transport.NewIO(NewIsNaN(), NewIsInf())),
		NewOr(transport.NewIO(core.From(false))),
		NewNot(transport.NewIO(core.From(false))),
	)
}
