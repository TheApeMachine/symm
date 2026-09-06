package temporal

import (
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewVelocity retains two observed records and composes a finite difference.
// Source yields float64 and clock yields int64 nanoseconds. Each is evaluated
// once per input. The first observation and non-advancing time have zero rate,
// with explicit definedness; the latest observation is still retained.
func NewVelocity(source, clock core.Primitive) core.Primitive {
	history := store.NewRetained(core.From([]core.Primitive{}))
	previous := collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0)))
	current := collection.NewAt[core.Primitive](transport.NewIO(core.From(1.0)))
	pair := transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(previous, store.NewKey("from")), transport.NewPipe(current, store.NewKey("through"))),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				equation.NewElapsed(
					transport.NewPipe(store.NewGet("from"), store.NewGet("at")),
					transport.NewPipe(store.NewGet("through"), store.NewGet("at")),
				),
				store.NewKey("elapsed"),
			),
			transport.NewPipe(
				equation.NewDifference[float64](
					transport.NewPipe(store.NewGet("through"), store.NewGet("value")),
					transport.NewPipe(store.NewGet("from"), store.NewGet("value")),
				),
				store.NewKey("difference"),
			),
			transport.NewPipe(store.NewConstant(core.From(true)), store.NewKey("has_prior")),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				equation.NewGreater[float64](store.NewGet("elapsed"), store.NewConstant(core.From(0.0))),
				store.NewKey("defined"),
			),
			transport.NewPipe(
				logic.NewGate(
					equation.NewGreater[float64](store.NewGet("elapsed"), store.NewConstant(core.From(0.0))),
					equation.NewRatio[float64](store.NewGet("difference"), store.NewGet("elapsed")),
					store.NewConstant(core.From(0.0)),
				),
				store.NewKey("rate"),
			),
		),
	)
	first := store.NewRecord(
		transport.NewPipe(collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0))), store.NewKey("through")),
		transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("elapsed")),
		transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("difference")),
		transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("rate")),
		transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("has_prior")),
		transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("defined")),
	)
	return transport.NewMap(
		transport.NewPipe(
			store.NewRecord(transport.NewPipe(source, store.NewKey("value")), transport.NewPipe(clock, store.NewKey("at"))),
			collection.NewAppend[core.Primitive](history),
			collection.NewTail[core.Primitive](transport.NewIO(core.From(2))),
			history,
			logic.NewGate(
				equation.NewGreater[float64](
					transport.NewPipe(transport.NewSpread[core.Primitive](), equation.NewCount()),
					store.NewConstant(core.From(1.0)),
				),
				pair,
				first,
			),
		),
	)
}
