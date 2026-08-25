package nomagique

import (
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Number is the keyed top-level composer. Every key owns one committed Frame, and
Step merges that frame with the incoming frame before running the pipeline, then
stores the result as the new committed state. The registry is safe for
concurrent keys; a small per-key lock serializes same-key writers.
*/
type Number[Key comparable] struct {
	primitive Primitive
	initial   func(Key) Frame
	streams   sync.Map
}

type numberStream struct {
	mutex sync.RWMutex
	frame Frame
}

// NewNumber composes primitives into one isolated numeric unit per key.
func NewNumber[Key comparable](primitives ...Primitive) *Number[Key] {
	return NewNumberWithInitial[Key](nil, primitives...)
}

// NewNumberWithInitial provides the initial committed state for newly seen keys.
func NewNumberWithInitial[Key comparable](
	initial func(Key) Frame,
	primitives ...Primitive,
) *Number[Key] {
	return &Number[Key]{
		primitive: Pipe(primitives...),
		initial:   initial,
	}
}

/*
Step merges the incoming frame over the key's committed frame, runs the
pipeline, and stores the result as the new committed state. The returned frame
is both the committed state and the step output; Err carries any validation
failure.
*/
func (number *Number[Key]) Step(key Key, input Frame) Frame {
	stream, err := number.stream(key)

	if err != nil {
		input.Err = err

		return input
	}

	stream.mutex.Lock()
	defer stream.mutex.Unlock()

	merged := stream.frame
	merged.Merge(input)
	output := Step(number.primitive, merged)

	if output.Err == nil {
		stream.frame = output
	}

	return output
}

// Project returns the committed state for one key.
func (number *Number[Key]) Project(key Key) (Frame, bool) {
	stream, found := number.load(key)

	if !found {
		return types.Frame{}, false
	}

	stream.mutex.RLock()
	state := stream.frame
	stream.mutex.RUnlock()

	return state, true
}

// Output returns the last successful output for one key (the committed state).
func (number *Number[Key]) Output(key Key) (Frame, bool) {
	return number.Project(key)
}

// Error returns the last transition error for one key.
func (number *Number[Key]) Error(key Key) (error, bool) {
	stream, found := number.load(key)

	if !found {
		return nil, false
	}

	stream.mutex.RLock()
	err := stream.frame.Err
	stream.mutex.RUnlock()

	return err, true
}

// Delete removes one keyed numeric unit.
func (number *Number[Key]) Delete(key Key) {
	if number == nil {
		return
	}

	number.streams.Delete(key)
}

// Reset replaces one key's committed state.
func (number *Number[Key]) Reset(key Key, initial Frame) error {
	stream, err := number.stream(key)

	if err != nil {
		return err
	}

	stream.mutex.Lock()
	stream.frame = initial
	stream.mutex.Unlock()

	return nil
}

// Range visits immutable copies of each committed keyed state.
func (number *Number[Key]) Range(yield func(Key, Frame) bool) {
	if number == nil || yield == nil {
		return
	}

	number.streams.Range(func(storedKey any, storedValue any) bool {
		key, validKey := storedKey.(Key)
		stream, validStream := storedValue.(*numberStream)

		if !validKey || !validStream {
			return true
		}

		stream.mutex.RLock()
		state := stream.frame
		stream.mutex.RUnlock()

		return yield(key, state)
	})
}

/*
CrossSection evaluates one committed focal state against every committed peer.
The pair callback receives the two committed frames as pointers so the caller
reads the retained bivariate series without copying a whole Frame per peer;
its output is folded serially through reduce (no goroutine-per-peer, no blocking
drain), then optionally finalized. The focal is snapshotted once under a brief
read lock and each peer read in place under its own brief read lock, so at most
one peer lock and a single focal lock are held at any instant, never a lock held
across the whole fold — concurrent CrossSections cannot deadlock.
*/
func (number *Number[Key]) CrossSection(
	key Key,
	pair func(focal *Frame, peer *Frame) Frame,
	reduce Primitive,
	finalize Primitive,
) (Frame, bool, error) {
	focalStream, found := number.load(key)

	if !found {
		return types.Frame{}, false, nil
	}

	// Snapshot the focal under a brief read lock. Per-symbol serialization (at
	// most one drain task per subscription key) means no other goroutine Steps
	// the focal while the peers are folded, so the copy is a stable view; taking
	// only a brief lock (never one held across the fold) keeps a hot focal from
	// stalling a peer's own Step behind another symbol's cross-section.
	focalStream.mutex.RLock()
	focal := focalStream.frame
	focalStream.mutex.RUnlock()

	accumulator := types.Frame{}
	reduced := false
	var crossSectionErr error

	number.streams.Range(func(storedKey any, storedValue any) bool {
		peerKey, validKey := storedKey.(Key)
		peerStream, validStream := storedValue.(*numberStream)

		if !validKey || !validStream || peerKey == key {
			return true
		}

		// Read the peer in place under its brief read lock and fold the pair
		// before releasing it, so the peer frame is never copied (a Frame is a
		// 66 KB value; a captured or value-copied peer escapes to the heap, which
		// was the dominant allocation source on this hot path). At most one peer
		// lock is held at a time, so concurrent CrossSections cannot deadlock.
		peerStream.mutex.RLock()
		out := pair(&focal, &peerStream.frame)
		peerStream.mutex.RUnlock()

		if out.Err != nil {
			crossSectionErr = out.Err
			return true
		}

		if !out.Finite() {
			crossSectionErr = errnie.Error(errnie.Err(
				errnie.Validation,
				"number pair output must be finite",
				nil,
			))
			return true
		}

		accumulator.Merge(out)
		accumulator = Step(reduce, accumulator)

		if accumulator.Err != nil {
			crossSectionErr = accumulator.Err
			return true
		}

		reduced = true

		return true
	})

	if crossSectionErr != nil || !reduced || finalize == nil {
		return accumulator, reduced, crossSectionErr
	}

	final := Step(finalize, accumulator)

	return final, true, final.Err
}

/*
ArgMax evaluates one score over every committed state and returns a unique
finite maximum only when it exceeds the exact cross-sectional median.
*/
func (number *Number[Key]) ArgMax(
	score Primitive,
	valueSymbol Symbol,
	readySymbol Symbol,
) (Key, float64, float64, bool, error) {
	var selected Key
	values := [MaxSlots]float64{}
	count := 0
	maximum := 0.0
	hasMaximum := false
	tied := false
	var selectionErr error

	number.Range(func(key Key, state Frame) bool {
		output := Step(score, state)

		if output.Err != nil {
			selectionErr = output.Err
			return false
		}

		ready, hasReady := output.Get(readySymbol)
		value, hasValue := output.Get(valueSymbol)

		if !hasReady || ready == 0 || !hasValue {
			return true
		}

		if !utils.IsFinite(ready) || !utils.IsFinite(value) {
			selectionErr = errnie.Error(errnie.Err(
				errnie.Validation,
				"number score must emit finite values",
				nil,
			))

			return false
		}

		if count >= len(values) {
			selectionErr = errnie.Error(errnie.Err(
				errnie.Validation,
				"number cross-section exceed Frame capacity",
				nil,
			))

			return false
		}

		values[count] = value
		count++

		if !hasMaximum || value > maximum {
			selected = key
			maximum = value
			hasMaximum = true
			tied = false
			return true
		}

		if value == maximum {
			tied = true
		}

		return true
	})

	if selectionErr != nil || count == 0 {
		return selected, 0, 0, false, selectionErr
	}

	median := exactMedian(&values, count)

	return selected, maximum, median, hasMaximum && !tied && maximum > median, nil
}

func exactMedian(values *[MaxSlots]float64, count int) float64 {
	middle := count / 2
	upper := selectValue(values[:count], middle)

	if count%2 != 0 {
		return upper
	}

	lower := selectValue(values[:count], middle-1)

	return lower + (upper-lower)/2
}

func selectValue(values []float64, target int) float64 {
	left := 0
	right := len(values) - 1

	for left < right {
		pivot := values[(left+right)/2]
		low := left
		high := right

		for low <= high {
			for low <= right && values[low] < pivot {
				low++
			}

			for high >= left && values[high] > pivot {
				high--
			}

			if low <= high {
				values[low], values[high] = values[high], values[low]
				low++
				high--
			}
		}

		if target <= high {
			right = high
			continue
		}

		if target >= low {
			left = low
			continue
		}

		return values[target]
	}

	return values[target]
}

func (number *Number[Key]) stream(key Key) (*numberStream, error) {
	if number == nil || number.primitive == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"number primitive is nil",
			nil,
		))
	}

	if stream, found := number.load(key); found {
		return stream, nil
	}

	initial := types.Frame{}

	if number.initial != nil {
		initial = number.initial(key)
	}

	candidate := &numberStream{frame: initial}
	stored, _ := number.streams.LoadOrStore(key, candidate)
	stream, valid := stored.(*numberStream)

	if !valid {
		return nil, types.PrimitiveError("number registry contains an invalid stream")
	}

	return stream, nil
}

func (number *Number[Key]) load(key Key) (*numberStream, bool) {
	if number == nil {
		return nil, false
	}

	stored, found := number.streams.Load(key)

	if !found {
		return nil, false
	}

	stream, valid := stored.(*numberStream)

	return stream, valid
}

// Single is an explicitly single-writer unkeyed numeric unit.
type Single func(input Frame) Frame

// NewSingle composes primitives into one state-carrying callable.
func NewSingle(primitives ...Primitive) Single {
	pipeline := Pipe(primitives...)
	state := types.Frame{}

	return func(input Frame) Frame {
		merged := state
		merged.Merge(input)
		output := Step(pipeline, merged)

		if output.Err == nil {
			state = output
		}

		return output
	}
}
