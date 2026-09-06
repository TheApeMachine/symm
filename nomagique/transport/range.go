package transport

import (
	"github.com/theapemachine/symm/nomagique/core"
	"math"
)

// Range enumerates [0,count). Its count is supplied by a Primitive; iteration
// state is private and never leaks into the data payload as a control opcode.
type Range struct {
	core.PrimitiveError
	count        core.Primitive
	index, total int
	open         bool
	current      core.Primitive
}

func NewRange(count core.Primitive) *Range { return &Range{count: count} }
func (sequence *Range) Next(core.Primitive) core.Primitive {
	if !sequence.open {
		core.Yield(
			NewIO(core.From(0.0)),
			sequence.count,
			func(_, count float64) float64 {
				if math.IsNaN(count) || math.IsInf(count, 0) || count < 0 || math.Trunc(count) != count || count >= float64(int(^uint(0)>>1)) {
					sequence.Error(core.ErrShape)
					return count
				}
				sequence.total = int(count)
				return count
			},
			sequence,
		)
		sequence.index = 0
		sequence.open = true
	}
	if sequence.Error() != nil {
		return nil
	}
	if sequence.index == sequence.total {
		sequence.open = false
		return nil
	}
	sequence.current = core.From(float64(sequence.index))
	sequence.index++
	return sequence.current
}
func (sequence *Range) Read() any { return core.To[any](sequence.current) }
