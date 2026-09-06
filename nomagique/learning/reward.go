package learning

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewReward measures cumulative objective changes and elapsed-time rates.
// Marks contain at (int64 Unix nanoseconds), version (uint64), value (float64).
// Identical redelivery is idempotent; invalid chronology cannot commit a new
// last/initial mark. A same-time newer version contributes reward, not time.
func NewReward() core.Primitive {
	memory := store.NewRetained(
		core.From(
			map[string]core.Primitive{
				"last": core.From(map[string]core.Primitive{"at": core.From(int64(0)), "version": core.From(uint64(0)), "value": core.From(0.0)}),
			},
		),
	)
	state := transport.NewPipe(store.NewKV[string](memory), memory)
	initial := store.NewRecord(
		transport.NewPipe(store.NewGet("mark"), store.NewKey("from")),
		transport.NewPipe(store.NewGet("mark"), store.NewKey("through")),
		transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("elapsed")),
		transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("reward")),
		transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("total_elapsed")),
		transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("total_reward")),
		transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("prior_rate")),
		transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("rate")),
		transport.NewPipe(store.NewConstant(core.From(0.0)), store.NewKey("differential")),
		transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("has_prior_rate")),
		transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("has_rate")),
		transport.NewPipe(store.NewConstant(core.From(uint64(0))), store.NewKey("transitions")),
	)
	first := transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(store.NewGet("mark"), store.NewKey("last")),
			transport.NewPipe(store.NewGet("mark"), store.NewKey("initial")),
			transport.NewPipe(initial, store.NewKey("outcome")),
		),
		state,
		store.NewGet("outcome"),
	)
	measured := transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(store.NewGet("last"), store.NewKey("from")),
			transport.NewPipe(store.NewGet("mark"), store.NewKey("through")),
			transport.NewPipe(
				equation.NewElapsed(
					transport.NewPipe(store.NewGet("last"), store.NewGet("at")),
					transport.NewPipe(store.NewGet("mark"), store.NewGet("at")),
				),
				store.NewKey("elapsed"),
			),
			transport.NewPipe(
				equation.NewElapsed(
					transport.NewPipe(store.NewGet("initial"), store.NewGet("at")),
					transport.NewPipe(store.NewGet("mark"), store.NewGet("at")),
				),
				store.NewKey("total_elapsed"),
			),
			transport.NewPipe(
				equation.NewDifference[float64](
					transport.NewPipe(store.NewGet("mark"), store.NewGet("value")),
					transport.NewPipe(store.NewGet("last"), store.NewGet("value")),
				),
				store.NewKey("reward"),
			),
			transport.NewPipe(
				equation.NewDifference[float64](
					transport.NewPipe(store.NewGet("mark"), store.NewGet("value")),
					transport.NewPipe(store.NewGet("initial"), store.NewGet("value")),
				),
				store.NewKey("total_reward"),
			),
			transport.NewPipe(store.NewGet("outcome"), store.NewGet("rate"), store.NewKey("prior_rate")),
			transport.NewPipe(store.NewGet("outcome"), store.NewGet("has_rate"), store.NewKey("has_prior_rate")),
			transport.NewPipe(
				equation.NewSum[uint64](
					transport.NewPipe(store.NewGet("outcome"), store.NewGet("transitions")),
					store.NewConstant(core.From(uint64(1))),
				),
				store.NewKey("transitions"),
			),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				equation.NewGreater[float64](store.NewGet("total_elapsed"), store.NewConstant(core.From(0.0))),
				store.NewKey("has_rate"),
			),
			transport.NewPipe(
				logic.NewGate(
					equation.NewGreater[float64](store.NewGet("total_elapsed"), store.NewConstant(core.From(0.0))),
					equation.NewRatio[float64](store.NewGet("total_reward"), store.NewGet("total_elapsed")),
					store.NewConstant(core.From(0.0)),
				),
				store.NewKey("rate"),
			),
			transport.NewPipe(
				logic.NewGate(
					store.NewGet("has_prior_rate"),
					equation.NewDifference[float64](
						store.NewGet("reward"),
						equation.NewProduct[float64](store.NewGet("prior_rate"), store.NewGet("elapsed")),
					),
					store.NewConstant(core.From(0.0)),
				),
				store.NewKey("differential"),
			),
		),
	)
	advance := transport.NewPipe(
		store.NewRecord(transport.NewPipe(), transport.NewPipe(measured, store.NewKey("outcome"))),
		store.NewRecord(transport.NewPipe(), transport.NewPipe(store.NewGet("mark"), store.NewKey("last"))),
		state,
		store.NewGet("outcome"),
	)
	duplicate := equation.NewAll(
		equation.NewEqual[uint64](
			transport.NewPipe(store.NewGet("mark"), store.NewGet("version")),
			transport.NewPipe(store.NewGet("last"), store.NewGet("version")),
		),
		equation.NewEqual[int64](
			transport.NewPipe(store.NewGet("mark"), store.NewGet("at")),
			transport.NewPipe(store.NewGet("last"), store.NewGet("at")),
		),
		equation.NewEqual[float64](
			transport.NewPipe(store.NewGet("mark"), store.NewGet("value")),
			transport.NewPipe(store.NewGet("last"), store.NewGet("value")),
		),
	)
	chronology := equation.NewAll(
		equation.NewGreater[uint64](
			transport.NewPipe(store.NewGet("mark"), store.NewGet("version")),
			transport.NewPipe(store.NewGet("last"), store.NewGet("version")),
		),
		equation.NewLessEqual[int64](
			transport.NewPipe(store.NewGet("last"), store.NewGet("at")),
			transport.NewPipe(store.NewGet("mark"), store.NewGet("at")),
		),
	)
	return transport.NewMap(
		logic.NewGate(
			equation.NewGreater[uint64](store.NewGet("version"), store.NewConstant(core.From(uint64(0)))),
			transport.NewPipe(
				store.NewKey("mark"),
				state,
				logic.NewGate(
					equation.NewEqual[uint64](
						transport.NewPipe(store.NewGet("last"), store.NewGet("version")),
						store.NewConstant(core.From(uint64(0))),
					),
					first,
					logic.NewGate(duplicate, store.NewGet("outcome"), logic.NewGate(chronology, advance, logic.NewReject(core.ErrDomain))),
				),
			),
			logic.NewReject(core.ErrDomain),
		),
	)
}
