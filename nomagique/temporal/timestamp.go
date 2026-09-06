package temporal

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"time"
)

// Timestamp converts a time.Time boundary payload to signed Unix nanoseconds.
// Holding a clock value is Retained's job, not another clock object model.
type Timestamp struct {
	core.PrimitiveError
	seed, current core.Primitive
}

func NewTimestamp() *Timestamp {
	return &Timestamp{seed: transport.NewIO(core.From(int64(0))), current: store.NewRetained(nil)}
}
func (stamp *Timestamp) Next(in core.Primitive) core.Primitive {
	result := core.Yield(stamp.seed, in, func(_ int64, value time.Time) int64 { return value.UnixNano() }, stamp)
	transport.NewDiscard().Next(transport.NewApply(stamp.current, transport.NewIO(result)))
	return result
}
func (stamp *Timestamp) Read() any { return core.To[any](stamp.current) }
