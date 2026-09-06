package logic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Pick selects between the held candidate and each arriving candidate through
// a configured predicate. An empty run yields an empty selection, not stale data.
// The result is a collection of zero or one Primitive; Spread delivers it.
type Pick struct {
	core.PrimitiveError
	predicate, seed, current core.Primitive
}

func NewPick(predicate core.Primitive) *Pick {
	return &Pick{current: store.NewRetained(nil), predicate: predicate, seed: transport.NewIO(core.From([]core.Primitive{}))}
}
func (pick *Pick) Next(in core.Primitive) core.Primitive {
	result := core.Yield(
		pick.seed,
		in,
		func(held []core.Primitive, value core.Primitive) []core.Primitive {
			if len(held) == 0 {
				return []core.Primitive{value}
			}
			selected := false
			core.Yield(
				transport.NewIO(core.From(false)),
				transport.NewApply(pick.predicate, transport.NewIO(core.From([]core.Primitive{held[0], value}))),
				func(_, v bool) bool { selected = v; return v },
				pick,
			)
			if selected {
				return []core.Primitive{value}
			}
			return held
		},
		pick,
	)
	transport.NewDiscard().Next(transport.NewApply(pick.current, transport.NewIO(result)))
	return result
}
func (pick *Pick) Read() any { return core.To[any](pick.current) }
