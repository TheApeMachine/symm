package correlation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewLagShape reads the neighbours of the explicitly selected index. It does
// not search for a different maximum (zero lag may exceed the nonzero winner).
// Undefined or out-of-range neighbours yield shape_defined=false, never a
// derivative left over from a previous profile. Coordinates are seconds.
func NewLagShape(profile, selected core.Primitive) core.Primitive {
	lower := transport.NewPipe(transport.NewApply(profile, nil), collection.NewAt[core.Primitive](
		transport.NewApply(equation.NewDifference[float64](store.NewGet("index"), store.NewConstant(core.From(1.0))), selected)))
	upper := transport.NewPipe(transport.NewApply(profile, nil), collection.NewAt[core.Primitive](
		transport.NewApply(equation.NewSum[float64](store.NewGet("index"), store.NewConstant(core.From(1.0))), selected)))
	left := transport.NewPipe(store.NewGet("lower"), store.NewGet("y"), calculus.NewAbsolute(transport.NewIO(core.From(0.0))))
	center := transport.NewPipe(store.NewGet("y"), calculus.NewAbsolute(transport.NewIO(core.From(0.0))))
	right := transport.NewPipe(store.NewGet("upper"), store.NewGet("y"), calculus.NewAbsolute(transport.NewIO(core.From(0.0))))
	absent := store.NewRecord(transport.NewPipe(),
		transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("shape_defined")))
	return logic.NewGate(equation.NewAll(
		equation.NewGreater[float64](store.NewGet("index"), store.NewConstant(core.From(0.0))),
		equation.NewLess[float64](store.NewGet("index"), equation.NewProduct[float64](store.NewGet("span"), store.NewConstant(core.From(2.0))))),
		transport.NewPipe(
			store.NewRecord(transport.NewPipe(), transport.NewPipe(lower, store.NewKey("lower")), transport.NewPipe(upper, store.NewKey("upper"))),
			logic.NewGate(equation.NewAll(
				transport.NewPipe(store.NewGet("lower"), store.NewGet("defined")),
				transport.NewPipe(store.NewGet("upper"), store.NewGet("defined"))),
				store.NewRecord(transport.NewPipe(),
					transport.NewPipe(store.NewConstant(core.From(true)), store.NewKey("shape_defined")),
					transport.NewPipe(equation.NewRatio[float64](equation.NewSecondDifference(left, center, right), store.NewConstant(core.From(2.0))), store.NewKey("prominence")),
					transport.NewPipe(equation.NewRatio[float64](
						equation.NewSecondDifference(left, center, right),
						transport.NewPipe(equation.NewProduct[float64](store.NewGet("spacing"), store.NewConstant(core.From(1e-9))),
							calculus.NewSquare(transport.NewIO(core.From(0.0))))), store.NewKey("curvature"))),
				absent)),
		absent)
}
