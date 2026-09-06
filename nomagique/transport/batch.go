package transport

import "github.com/theapemachine/symm/nomagique/core"

// Batch is a source that inserts delivery boundaries into a configured source.
// Reaching its quota does not step or discard the next upstream value. A later
// caller can continue from that value. An upstream nil is forwarded unchanged.
// Limit is a source of one positive integer per batch, not a domain-specific mode.
type Batch struct {
	core.PrimitiveError
	source, limit, current core.Primitive
	remaining              int
}

func NewBatch(source, limit core.Primitive) *Batch {
	return &Batch{source: source, limit: limit, remaining: -1}
}
func (batch *Batch) Next(core.Primitive) core.Primitive {
	if batch.remaining == 0 {
		batch.remaining = -1
		return nil
	}
	if batch.remaining < 0 {
		observed := 0
		core.Yield(NewIO(core.From(0)), batch.limit, func(_ int, size int) int {
			batch.remaining = size
			observed++
			return size
		}, batch)
		if observed != 1 || batch.remaining < 1 {
			batch.Error(core.ErrShape)
			return nil
		}
	}
	value := batch.source.Next(nil)
	batch.Error(batch.source.Error())
	if value == nil {
		batch.remaining = -1
		return nil
	}
	batch.remaining--
	batch.current = value
	return value
}
func (batch *Batch) Read() any { return core.To[any](batch.current) }
