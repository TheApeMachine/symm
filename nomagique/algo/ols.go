package algo

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/vector"
)

// NewOLS composes ordinary least squares via the supplied source's normal
// equations, with partial-pivot Gauss-Jordan solving replacing its LU backend.
// Input is {x: [][]float64, y: []float64}; callers include an intercept column
// explicitly. n<=p and rank deficiency yield defined=false, empty coefficients,
// and NaN residual variance. There is no ridge or invented rank tolerance.
// The configured tolerance expression is forwarded to the linear solver.
func NewOLS(tolerance core.Primitive) core.Primitive {
	memory := store.NewRetained(core.From(map[string]core.Primitive{}))
	state := transport.NewPipe(store.NewKV[string](memory), memory)
	parameters := transport.NewApply(store.NewGet("parameters"), memory)
	prepare := store.NewRecord(
		transport.NewPipe(),
		transport.NewPipe(store.NewGet("x"), transport.NewSpread[[]float64](), equation.NewCount(), store.NewKey("observations")),
		transport.NewPipe(
			store.NewGet("x"),
			matrix.NewTranspose[float64](),
			transport.NewSpread[[]float64](),
			equation.NewCount(),
			store.NewKey("parameters"),
		),
		transport.NewPipe(store.NewGet("y"), transport.NewSpread[float64](), equation.NewCount(), store.NewKey("outcomes")),
		transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("defined")),
		transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("rank")),
		transport.NewPipe(store.NewConstant(core.From([]float64{})), store.NewKey("coefficients")),
		transport.NewPipe(store.NewConstant(core.From([]float64{})), store.NewKey("coefficient_variance")),
		transport.NewPipe(
			equation.NewRatio[float64](store.NewConstant(core.From(0.0)), store.NewConstant(core.From(0.0))),
			store.NewKey("residual_variance"),
		),
		transport.NewPipe(
			equation.NewRatio[float64](store.NewConstant(core.From(0.0)), store.NewConstant(core.From(0.0))),
			store.NewKey("residual_sse"),
		),
	)
	cross := store.NewRecord(
		transport.NewPipe(),
		transport.NewPipe(
			matrix.NewProduct(transport.NewPipe(store.NewGet("x"), matrix.NewTranspose[float64]()), store.NewGet("x")),
			store.NewKey("xtx"),
		),
		transport.NewPipe(
			matrix.NewProduct(
				transport.NewPipe(store.NewGet("x"), matrix.NewTranspose[float64]()),
				transport.NewPipe(store.NewGet("y"), transport.NewSpread[float64](), matrix.NewColumn()),
			),
			store.NewKey("xty"),
		),
		transport.NewPipe(store.NewGet("y"), transport.NewSpread[float64](), equation.NewEnergy(), store.NewKey("yty")),
	)
	solve := transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(store.NewGet("xtx"), store.NewKey("left")),
			transport.NewPipe(
				matrix.NewAugment(store.NewGet("xty"), transport.NewPipe(store.NewGet("parameters"), matrix.NewIdentity())),
				store.NewKey("right"),
			),
		),
		NewGaussJordan(tolerance),
		store.NewRecord(
			transport.NewPipe(store.NewGet("solution"), store.NewKey("solution")),
			transport.NewPipe(store.NewGet("rank"), store.NewKey("rank")),
			transport.NewPipe(store.NewGet("defined"), store.NewKey("defined")),
		),
		state,
	)
	coefficients := transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				store.NewGet("solution"),
				transport.NewSpread[[]float64](),
				transport.NewMap(collection.NewAt[float64](transport.NewIO(core.From(0.0)))),
				transport.NewCollect[float64](),
				store.NewKey("coefficients"),
			),
			transport.NewPipe(
				store.NewGet("solution"),
				transport.NewSpread[[]float64](),
				transport.NewMap(collection.NewTail[float64](transport.NewPipe(parameters, calculus.NewConvert[float64, int]()))),
				transport.NewCollect[[]float64](),
				store.NewKey("inverse"),
			),
		),
		state,
	)
	residual := transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				equation.NewDifference[float64](
					store.NewGet("yty"),
					vector.NewDot(
						transport.NewPipe(store.NewGet("coefficients"), transport.NewSpread[float64]()),
						transport.NewPipe(store.NewGet("xty"), transport.NewSpread[[]float64](), transport.NewSpread[float64]()),
					),
				),
				calculus.NewMaximum(transport.NewIO(core.From(0.0))),
				store.NewKey("residual_sse"),
			),
		),
		state,
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				equation.NewRatio[float64](
					store.NewGet("residual_sse"),
					equation.NewDifference[float64](store.NewGet("observations"), store.NewGet("parameters")),
				),
				store.NewKey("residual_variance"),
			),
		),
		state,
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				vector.NewScale(
					transport.NewPipe(store.NewGet("inverse"), matrix.NewDiagonal()), store.NewGet("residual_variance")),
				store.NewKey("coefficient_variance"),
			),
		),
	)
	fit := transport.NewPipe(
		cross,
		state,
		solve,
		logic.NewGate(store.NewGet("defined"), transport.NewPipe(coefficients, residual), transport.NewPipe()),
	)
	return transport.NewPipe(
		prepare,
		state,
		logic.NewGate(
			equation.NewAll(
				equation.NewEqual[float64](store.NewGet("observations"), store.NewGet("outcomes")),
				transport.NewPipe(store.NewGet("x"), matrix.NewFinite()),
				transport.NewPipe(store.NewGet("y"), transport.NewSpread[float64](), vector.NewFinite()),
			),
			logic.NewGate(
				equation.NewAll(
					equation.NewGreater[float64](store.NewGet("parameters"), store.NewConstant(core.From(0.0))),
					equation.NewGreater[float64](store.NewGet("observations"), store.NewGet("parameters")),
				),
				fit,
				transport.NewPipe(),
			),
			logic.NewReject(core.ErrShape),
		),
	)
}
