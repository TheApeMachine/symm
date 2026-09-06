package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewMinimum binds both expressions to one captured input. The calculus fold owns
// the comparison and IEEE behavior; no finite sentinel substitutes for infinity.
func NewMinimum(left, right core.Primitive) core.Primitive {
	input := store.NewRetained(nil)
	return transport.NewPipe(
		input,
		transport.NewApply(calculus.NewMinimum(transport.NewApply(left, input)), transport.NewApply(right, input)),
	)
}
