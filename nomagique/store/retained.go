package store

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Retained holds the latest Primitive reference. The configured initial value
// exists before the first update. Asking with nil yields this retained view;
// delivery itself is owned by IO, not a special nil-input branch here.
type Retained struct {
	core.PrimitiveError
	held     core.Primitive
	delivery core.Primitive
}

func NewRetained(initial core.Primitive) *Retained {
	retained := &Retained{held: initial}
	retained.delivery = transport.NewIO(retained)
	return retained
}
func (retained *Retained) Next(in core.Primitive) core.Primitive {
	return core.Yield(
		retained.delivery,
		in,
		func(_ core.Primitive, value core.Primitive) core.Primitive {
			retained.held = value
			return value
		},
		retained,
	)
}
func (retained *Retained) Read() any { return core.To[any](retained.held) }
