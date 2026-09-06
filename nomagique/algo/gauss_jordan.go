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

// NewGaussJordan composes partial-pivot Gauss-Jordan elimination. Input is a
// record containing left (square [][]float64) and right (n-by-q [][]float64).
// The tolerance expression supplies an absolute pivot floor. A singular solve
// yields defined=false and no solution, not regularization or guessed values.
// Numeric work belongs to dot/scale/difference compositions; this constructor
// owns only their ordering and feedback. It never operates on Go matrices.
func NewGaussJordan(tolerance core.Primitive) core.Primitive {
	memory := store.NewRetained(core.From(map[string]core.Primitive{}))
	state := transport.NewPipe(store.NewKV[string](memory), memory)
	rows := transport.NewApply(store.NewGet("rows"), memory)
	pivot := transport.NewApply(store.NewGet("pivot"), memory)
	row := transport.NewApply(store.NewGet("row"), memory)
	winner := transport.NewApply(store.NewGet("pivot_row"), memory)

	prepare := store.NewRecord(
		transport.NewPipe(),
		transport.NewPipe(store.NewGet("left"), transport.NewSpread[[]float64](), equation.NewCount(), store.NewKey("rows")),
		transport.NewPipe(
			store.NewGet("left"),
			matrix.NewTranspose[float64](),
			transport.NewSpread[[]float64](),
			equation.NewCount(),
			store.NewKey("columns"),
		),
		transport.NewPipe(
			store.NewGet("right"),
			matrix.NewTranspose[float64](),
			transport.NewSpread[[]float64](),
			equation.NewCount(),
			store.NewKey("right_columns"),
		),
		transport.NewPipe(matrix.NewAugment(store.NewGet("left"), store.NewGet("right")), store.NewKey("matrix")),
		transport.NewPipe(tolerance, store.NewKey("tolerance")),
		transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("rank")),
		transport.NewPipe(store.NewConstant(core.From(true)), store.NewKey("defined")),
	)

	choose := transport.NewPipe(
		store.NewGet("matrix"),
		matrix.NewTranspose[float64](),
		collection.NewAt[[]float64](pivot),
		collection.NewTail[float64](
			transport.NewApply(
				transport.NewPipe(
					equation.NewDifference[float64](store.NewGet("rows"), store.NewGet("pivot")),
					calculus.NewConvert[float64, int](),
				),
				memory,
			),
		),
		transport.NewSpread[float64](),
		transport.NewMap(calculus.NewAbsolute(transport.NewIO(core.From(0.0)))),
		equation.NewArgmax(),
		store.NewRecord(
			transport.NewPipe(
				equation.NewSum[float64](store.NewGet("index"), transport.NewApply(store.NewGet("pivot"), memory)),
				store.NewKey("pivot_row"),
			),
			transport.NewPipe(store.NewGet("value"), store.NewKey("pivot_magnitude")),
		),
		state,
	)

	swap := transport.NewPipe(store.NewGet("matrix"), collection.NewSwap[[]float64](pivot, winner), store.NewKey("matrix"), state)
	normalize := transport.NewPipe(
		store.NewGet("matrix"),
		collection.NewSet[[]float64](
			pivot,
			transport.NewApply(
				vector.NewScale(
					transport.NewPipe(store.NewGet("matrix"), collection.NewAt[[]float64](pivot)),
					equation.NewRatio[float64](
						store.NewConstant(core.From(1.0)),
						transport.NewPipe(store.NewGet("matrix"), collection.NewAt[[]float64](pivot), collection.NewAt[float64](pivot)),
					),
				),
				memory,
			),
		),
		store.NewKey("matrix"),
		state,
	)

	eliminate := transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				store.NewGet("matrix"),
				collection.NewAt[[]float64](row),
				collection.NewAt[float64](pivot),
				store.NewKey("factor"),
			),
		),
		state,
		store.NewGet("matrix"),
		collection.NewSet[[]float64](
			row,
			transport.NewApply(
				vector.NewDifference(
					transport.NewPipe(store.NewGet("matrix"), collection.NewAt[[]float64](row), transport.NewSpread[float64]()),
					transport.NewPipe(
						vector.NewScale(transport.NewPipe(store.NewGet("matrix"), collection.NewAt[[]float64](pivot)), store.NewGet("factor")),
						transport.NewSpread[float64](),
					),
				),
				memory,
			),
		),
		store.NewKey("matrix"),
		state,
	)
	eliminateRows := transport.NewPipe(
		transport.NewApply(
			transport.NewMap(
				transport.NewPipe(
					store.NewKey("row"),
					state,
					logic.NewGate(equation.NewEqual[float64](store.NewGet("row"), store.NewGet("pivot")),
						transport.NewPipe(), eliminate),
				),
			),
			transport.NewRange(rows),
		),
		transport.NewDiscard(),
		transport.NewApply(memory, nil),
	)

	advance := transport.NewPipe(
		swap,
		normalize,
		eliminateRows,
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(equation.NewSum[float64](store.NewGet("rank"), store.NewConstant(core.From(1.0))), store.NewKey("rank")),
		),
		state,
	)
	deficient := transport.NewPipe(
		store.NewRecord(transport.NewPipe(), transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("defined"))),
		state,
	)
	step := transport.NewPipe(
		store.NewKey("pivot"),
		state,
		logic.NewGate(
			store.NewGet("defined"),
			transport.NewPipe(
				choose,
				logic.NewGate(equation.NewGreater[float64](store.NewGet("pivot_magnitude"), store.NewGet("tolerance")), advance, deficient),
			),
			transport.NewPipe(),
		),
	)

	solve := transport.NewPipe(
		transport.NewApply(transport.NewMap(step), transport.NewRange(rows)),
		transport.NewDiscard(),
		transport.NewApply(memory, nil),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				logic.NewGate(
					store.NewGet("defined"),
					transport.NewPipe(
						store.NewGet("matrix"),
						transport.NewSpread[[]float64](),
						transport.NewMap(
							collection.NewTail[float64](
								transport.NewApply(transport.NewPipe(store.NewGet("right_columns"), calculus.NewConvert[float64, int]()), memory),
							),
						),
						transport.NewCollect[[]float64](),
					),
					store.NewConstant(core.From([][]float64{})),
				),
				store.NewKey("solution"),
			),
		),
	)
	return transport.NewPipe(
		prepare,
		state,
		logic.NewGate(
			equation.NewAll(
				transport.NewPipe(store.NewGet("left"), matrix.NewFinite()),
				transport.NewPipe(store.NewGet("right"), matrix.NewFinite()),
				transport.NewPipe(store.NewGet("tolerance"), logic.NewFinite()),
				equation.NewGreater[float64](store.NewGet("rows"), store.NewConstant(core.From(0.0))),
				equation.NewEqual[float64](store.NewGet("rows"), store.NewGet("columns")),
				equation.NewGreater[float64](store.NewGet("right_columns"), store.NewConstant(core.From(0.0))),
				equation.NewLessEqual[float64](store.NewConstant(core.From(0.0)), store.NewGet("tolerance")),
			),
			solve,
			logic.NewReject(core.ErrShape),
		),
	)
}
