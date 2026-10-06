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
	generation := &ArenaGeneration{
		allocator: NewAllocator(),
	}
	generation.refs.Store(1)
	return generation
}

func (generation *ArenaGeneration) Retain() {
	if generation == nil {
		return
	}

	generation.refs.Add(1)
}

func (generation *ArenaGeneration) Release() {
	if generation == nil {
		return
	}

	if generation.refs.Add(-1) == 0 && generation.sealed.Load() {
		generation.free()
	}
}

func (generation *ArenaGeneration) Seal() {
	if generation == nil {
		return
	}

	generation.sealed.Store(true)

	if generation.refs.Load() <= 0 {
		generation.free()
	}
}

func (generation *ArenaGeneration) free() {
	if generation.freed.CompareAndSwap(false, true) && generation.allocator != nil {
		Free(generation.allocator)
		generation.allocator = nil
	}
}

func (generation *ArenaGeneration) IsFreed() bool {
	if generation == nil {
		return true
	}

	return generation.freed.Load()
}

func (generation *ArenaGeneration) Allocator() Allocator {
	if generation == nil {
		return nil
	}

	return generation.allocator
}

func (generation *ArenaGeneration) RefCount() int64 {
	if generation == nil {
		return 0
	}

	return generation.refs.Load()
}

type generationEntry struct {
	gen    *ArenaGeneration
	endSeq int64
}

/*
ArenaOwner manages rotating arena generations for one producer node.
Each producer is sequential with respect to its own Step stream, so ArenaOwner
does not require a mutex merely to allocate.
*/
type ArenaOwner struct {
	source      string
	current     *ArenaGeneration
	previous    *ArenaGeneration
	generations []generationEntry
	capacity    int
	window      int
	count       int
	currentGen  int64
	seqMode     bool
}

func NewArenaOwner(source string, capacity ...int) *ArenaOwner {
	capVal := 1024
	if len(capacity) > 0 && capacity[0] > 0 {
		capVal = capacity[0]
	}

	windowVal := capVal
	if len(capacity) > 1 && capacity[1] > 0 {
		windowVal = capacity[1]
	}

	current := NewArenaGeneration()

	return &ArenaOwner{
		source:      source,
		current:     current,
		capacity:    capVal,
		window:      windowVal,
		generations: []generationEntry{{gen: current, endSeq: int64(capVal - 1)}},
	}
}

func (owner *ArenaOwner) SetWindow(window int) {
	if owner == nil || window <= 0 {
		return
	}

	owner.window = window
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
	if owner == nil || owner.capacity <= 0 {
		return
	}

	owner.seqMode = true

	genIdx := seq / int64(owner.capacity)
	if len(owner.generations) == 0 {
		owner.currentGen = genIdx
		owner.generations = append(owner.generations, generationEntry{
			gen:    owner.current,
			endSeq: (genIdx+1)*int64(owner.capacity) - 1,
		})
	}

	if genIdx > owner.currentGen {
		owner.currentGen = genIdx
		owner.previous = owner.current
		owner.current = NewArenaGeneration()
		owner.generations = append(owner.generations, generationEntry{
			gen:    owner.current,
			endSeq: (genIdx+1)*int64(owner.capacity) - 1,
		})
	}

	window := int64(owner.window)
	if window < int64(owner.capacity) {
		window = int64(owner.capacity)
	}

	safeMargin := window + int64(owner.capacity)

	retained := owner.generations[:0]
	for _, entry := range owner.generations {
		if entry.endSeq < seq-safeMargin {
			entry.gen.Seal()
			entry.gen.Release()
			continue
		}

		retained = append(retained, entry)
	}

	owner.generations = retained

	if owner.previous != nil && owner.previous.IsFreed() {
		owner.previous = nil
	}
}

func (owner *ArenaOwner) Close() {
	if owner.seqMode {
		for _, entry := range owner.generations {
			entry.gen.Seal()
			entry.gen.Release()
		}
		owner.generations = nil
		owner.current = nil
		owner.previous = nil
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
func (owner *ArenaOwner) NewMeasurement(
	epoch int64,
	label string,
	source string,
	seqIdx int64,
	tick int64,
	peers []*Measurement,
	metadata ...StringEntry,
) *Measurement {
	gen := owner.current
	alloc := gen.Allocator()

	measurement := New[Measurement](alloc)
	measurement.Epoch = epoch
	measurement.Label = label
	measurement.Source = source
	measurement.SeqIdx = seqIdx
	measurement.Tick = tick
	measurement.peers = peers
	measurement.metadata = metadata


	if !owner.seqMode {
		owner.count++
		if owner.capacity > 0 && owner.count >= owner.capacity {
			owner.Rotate()
		}
	}

	return measurement
}

/*
Publication carries a WORM Measurement pointer together with its arena generation
lifetime token across transport boundaries.
*/
type Publication struct {
	Measurement *Measurement
	Generation  *ArenaGeneration
}

func NewPublication(measurement *Measurement, gen *ArenaGeneration) Publication {
	return Publication{
		Measurement: measurement,
		Generation:  gen,
	}
}

func (pub Publication) Retain() {
	if pub.Generation != nil {
		pub.Generation.Retain()
	}
}

func (pub Publication) Release() {
	if pub.Generation != nil {
		pub.Generation.Release()
	}
}
