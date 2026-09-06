package correlation

import (
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewPath retains timestamped observation records. An equal timestamp restates
// the last observation; a regression yields accepted=false without editing the
// path. Optional collection-to-collection retention is supplied by composition.
// Input: {at:int64, value:float64}. Output contains observations ([]Primitive),
// accepted, restated, count, has_span, from and to. It has no Observe/Len side API.
func NewPath(retention ...core.Primitive) core.Primitive {
	memory := store.NewRetained(core.From(map[string]core.Primitive{
		"observations": core.From([]core.Primitive{}),
	}))
	context := store.NewRetained(nil)
	incoming := store.NewRetained(nil)
	// Store a record snapshot, not the live retention connection itself.
	observation := transport.NewApply(store.NewRecord(transport.NewPipe()), incoming)
	lastIndex := transport.NewApply(equation.NewDifference[float64](
		store.NewGet("prior_count"), store.NewConstant(core.From(1.0))), context)
	lastTime := transport.NewPipe(store.NewGet("observations"),
		collection.NewAt[core.Primitive](lastIndex), store.NewGet("at"))

	appendValue := transport.NewPipe(
		observation,
		collection.NewAppend[core.Primitive](transport.NewApply(store.NewGet("observations"), context)),
	)
	replaceValue := transport.NewPipe(
		store.NewGet("observations"),
		collection.NewSet[core.Primitive](lastIndex, observation),
	)
	accept := transport.NewPipe(
		store.NewRecord(transport.NewPipe(),
			transport.NewPipe(
				logic.NewGate(store.NewGet("restated"), replaceValue, appendValue),
				transport.NewPipe(retention...), store.NewKey("observations"))),
		store.NewRecord(transport.NewPipe(),
			transport.NewPipe(store.NewConstant(core.From(true)), store.NewKey("accepted"))),
	)
	reject := store.NewRecord(transport.NewPipe(),
		transport.NewPipe(store.NewConstant(core.From(false)), store.NewKey("accepted")))
	return transport.NewMap(logic.NewGate(
		equation.NewAll(store.NewHas("at"), store.NewHas("value")),
		transport.NewPipe(
			incoming,
			store.NewKV[string](memory),
			store.NewRecord(transport.NewPipe(),
				transport.NewPipe(store.NewGet("observations"), transport.NewSpread[core.Primitive](),
					equation.NewCount(), store.NewKey("prior_count"))),
			context,
			store.NewRecord(transport.NewPipe(),
				transport.NewPipe(logic.NewGate(
					equation.NewGreater[float64](store.NewGet("prior_count"), store.NewConstant(core.From(0.0))),
					equation.NewEqual[int64](store.NewGet("at"), lastTime),
					store.NewConstant(core.From(false))), store.NewKey("restated"))),
			logic.NewGate(
				equation.NewEqual[float64](store.NewGet("prior_count"), store.NewConstant(core.From(0.0))),
				accept,
				logic.NewGate(equation.NewLessEqual[int64](lastTime, store.NewGet("at")), accept, reject)),
			store.NewRecord(transport.NewPipe(),
				transport.NewPipe(store.NewGet("observations"), transport.NewSpread[core.Primitive](),
					equation.NewCount(), store.NewKey("count"))),
			context,
			store.NewRecord(transport.NewPipe(),
				transport.NewPipe(equation.NewGreater[float64](store.NewGet("count"), store.NewConstant(core.From(0.0))),
					store.NewKey("has_span"))),
			store.NewRecord(transport.NewPipe(),
				transport.NewPipe(logic.NewGate(store.NewGet("has_span"),
					transport.NewPipe(store.NewGet("observations"),
						collection.NewAt[core.Primitive](transport.NewIO(core.From(0.0))), store.NewGet("at")),
					store.NewConstant(core.From(int64(0)))), store.NewKey("from")),
				transport.NewPipe(logic.NewGate(store.NewGet("has_span"),
					transport.NewPipe(store.NewGet("observations"), collection.NewAt[core.Primitive](
						transport.NewApply(equation.NewDifference[float64](store.NewGet("count"),
							store.NewConstant(core.From(1.0))), context)), store.NewGet("at")),
					store.NewConstant(core.From(int64(0)))), store.NewKey("to"))),
			memory,
		),
		logic.NewReject(core.ErrShape),
	))
}
