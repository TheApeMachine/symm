package data

import (
	"sync/atomic"
)

/*
ArenaGeneration manages the lifetime of one bounded generation of arena allocations.
It starts with an active reference for the synchronous pipeline (refs = 1).
When async readers (e.g. StoreTee, UITee) retain publications, refs increments.
When readers finish their final dereference, refs decrements.
Once the synchronous pipeline moves past this generation, Seal() and Release() are
called. When both the generation is sealed and all readers have released, the underlying
arena is freed exactly once.
*/
type ArenaGeneration struct {
	allocator Allocator
	refs      atomic.Int64
	sealed    atomic.Bool
	freed     atomic.Bool
}

func NewArenaGeneration() *ArenaGeneration {
	g := &ArenaGeneration{
		allocator: NewAllocator(),
	}
	g.refs.Store(1)
	return g
}

func (g *ArenaGeneration) Retain() {
	if g == nil {
		return
	}
	g.refs.Add(1)
}

func (g *ArenaGeneration) Release() {
	if g == nil {
		return
	}
	if g.refs.Add(-1) == 0 {
		if g.sealed.Load() {
			g.free()
		}
	}
}

func (g *ArenaGeneration) Seal() {
	if g == nil {
		return
	}
	g.sealed.Store(true)
	if g.refs.Load() <= 0 {
		g.free()
	}
}

func (g *ArenaGeneration) free() {
	if g.freed.CompareAndSwap(false, true) {
		if g.allocator != nil {
			Free(g.allocator)
			g.allocator = nil
		}
	}
}

func (g *ArenaGeneration) IsFreed() bool {
	if g == nil {
		return true
	}
	return g.freed.Load()
}

func (g *ArenaGeneration) Allocator() Allocator {
	if g == nil {
		return nil
	}
	return g.allocator
}

func (g *ArenaGeneration) RefCount() int64 {
	if g == nil {
		return 0
	}
	return g.refs.Load()
}

/*
ArenaOwner manages rotating arena generations for one producer node.
Each producer is sequential with respect to its own Step stream, so ArenaOwner
does not require a mutex merely to allocate.
*/
type ArenaOwner struct {
	current  *ArenaGeneration
	previous *ArenaGeneration
	capacity int
	count    int
}

func NewArenaOwner(capacity ...int) *ArenaOwner {
	capVal := 1024
	if len(capacity) > 0 && capacity[0] > 0 {
		capVal = capacity[0]
	}

	return &ArenaOwner{
		current:  NewArenaGeneration(),
		capacity: capVal,
	}
}

func (owner *ArenaOwner) CurrentGeneration() *ArenaGeneration {
	if owner == nil {
		return nil
	}
	return owner.current
}

func (owner *ArenaOwner) Rotate() {
	if owner == nil {
		return
	}

	old := owner.previous
	if old != nil {
		old.Seal()
		old.Release()
	}

	owner.previous = owner.current
	owner.current = NewArenaGeneration()
	owner.count = 0
}

func (owner *ArenaOwner) Advance(seq int64) {
	if owner == nil {
		return
	}

	if seq > 0 && owner.capacity > 0 && seq%int64(owner.capacity) == 0 {
		owner.Rotate()
	}
}

func (owner *ArenaOwner) Close() {
	if owner == nil {
		return
	}

	if owner.current != nil {
		owner.current.Seal()
		owner.current.Release()
		owner.current = nil
	}

	if owner.previous != nil {
		owner.previous.Seal()
		owner.previous.Release()
		owner.previous = nil
	}
}

/*
NewMeasurement allocates a fresh Measurement from the current generation.
Its slices (Metrics, Metadata, Provenance, Peers) are initialized from the arena,
avoiding GC heap map allocations on the hot path.
*/
func (owner *ArenaOwner) NewMeasurement(source string) *Measurement[float64] {
	if owner == nil {
		return NewMeasurement[float64](source)
	}

	gen := owner.current
	alloc := gen.Allocator()

	m := New[Measurement[float64]](alloc)
	m.ID = -1
	m.Source = source
	m.Metrics = MakeSlice[MetricEntry[float64]](alloc, 0, 16)
	m.Metadata = MakeSlice[StringEntry](alloc, 0, 8)
	m.Provenance = MakeSlice[StringEntry](alloc, 0, 8)
	m.Peers = MakeSlice[*Measurement[float64]](alloc, 0, 4)

	owner.count++
	if owner.capacity > 0 && owner.count >= owner.capacity {
		owner.Rotate()
	}

	return m
}

/*
Publication carries a WORM Measurement pointer together with its arena generation
lifetime token across transport boundaries.
*/
type Publication struct {
	Measurement *Measurement[float64]
	Generation  *ArenaGeneration
}

func NewPublication(m *Measurement[float64], gen *ArenaGeneration) Publication {
	return Publication{
		Measurement: m,
		Generation:  gen,
	}
}

func (p Publication) Retain() {
	if p.Generation != nil {
		p.Generation.Retain()
	}
}

func (p Publication) Release() {
	if p.Generation != nil {
		p.Generation.Release()
	}
}

