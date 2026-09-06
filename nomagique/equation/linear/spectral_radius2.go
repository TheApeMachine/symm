package linear

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewSpectralRadius2 composes the quadratic roots of a 2-by-2 matrix. For a
// complex-conjugate pair the common modulus is sqrt(det); otherwise the
// largest absolute real root is selected. Input entries are a,b,c,d.
func NewSpectralRadius2() core.Primitive {
	return transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(equation.NewSum[float64](store.NewGet("a"), store.NewGet("d")), store.NewKey("trace")),
			transport.NewPipe(NewDeterminant2(), store.NewKey("determinant")),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				equation.NewDifference[float64](
					transport.NewPipe(store.NewGet("trace"), calculus.NewSquare(transport.NewIO(core.From(0.0)))),
					equation.NewProduct[float64](store.NewConstant(core.From(4.0)), store.NewGet("determinant")),
				),
				store.NewKey("discriminant"),
			),
		),
		logic.NewGate(
			equation.NewLess[float64](store.NewGet("discriminant"), store.NewConstant(core.From(0.0))),
			transport.NewPipe(store.NewGet("determinant"), calculus.NewSqrt(transport.NewIO(core.From(0.0)))),
			transport.NewPipe(
				transport.NewFan(
					transport.NewPipe(),
					transport.NewIO(
						transport.NewPipe(
							equation.NewRatio[float64](
								equation.NewSum[float64](
									store.NewGet("trace"),
									transport.NewPipe(store.NewGet("discriminant"), calculus.NewSqrt(transport.NewIO(core.From(0.0)))),
								),
								store.NewConstant(core.From(2.0)),
							),
							calculus.NewAbsolute(transport.NewIO(core.From(0.0))),
						),
						transport.NewPipe(
							equation.NewRatio[float64](
								equation.NewDifference[float64](
									store.NewGet("trace"),
									transport.NewPipe(store.NewGet("discriminant"), calculus.NewSqrt(transport.NewIO(core.From(0.0)))),
								),
								store.NewConstant(core.From(2.0)),
							),
							calculus.NewAbsolute(transport.NewIO(core.From(0.0))),
						),
					),
				),
				calculus.NewMaximum(transport.NewIO(core.From(0.0))),
			),
		),
	)
}
