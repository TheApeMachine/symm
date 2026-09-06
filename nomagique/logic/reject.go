package logic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Reject is explicit refusal. It does not hide bad input by emitting an old or
// zero-valued measurement.
type Reject struct {
	core.PrimitiveError
	reason error
}

func NewReject(reason error) *Reject { return &Reject{reason: reason} }
func (reject *Reject) Next(in core.Primitive) core.Primitive {
	reject.Error(reject.reason)
	core.Yield(transport.NewIO(core.From(0)), in, func(n int, _ core.Primitive) int { return n }, reject)
	return nil
}
func (reject *Reject) Read() any { return nil }
