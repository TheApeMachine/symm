package nomagique

import (
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Number is the keyed top-level composer. Every key owns one isolated Stream.
The registry is safe for concurrent keys, and a small per-key lock serializes
same-key writers without allocating a new Frame snapshot on every transition.
*/
type Number[Key comparable] struct {
	primitive Primitive
	initial   func(Key) Frame
	streams   sync.Map
}

type numberStream struct {
	mutex  sync.RWMutex
	stream Stream
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

// Step applies one input to its keyed pipeline and commits the transition.
func (number *Number[Key]) Step(key Key, input Frame) (Frame, error) {
	stream, err := number.stream(key)

	if err != nil {
		return Frame{}, err
	}

	stream.mutex.Lock()
	defer stream.mutex.Unlock()

	return stream.stream.Step(input)
}

// Project returns the committed state for one key.
func (number *Number[Key]) Project(key Key) (Frame, bool) {
	stream, found := number.load(key)

	if !found {
		return Frame{}, false
	}

	stream.mutex.RLock()
	state := stream.stream.Project()
	stream.mutex.RUnlock()

	return state, true
}

// Output returns the last successful output for one key.
func (number *Number[Key]) Output(key Key) (Frame, bool) {
	stream, found := number.load(key)

	if !found {
		return Frame{}, false
	}

	stream.mutex.RLock()
	output := stream.stream.Output()
	stream.mutex.RUnlock()

	return output, true
}

// Error returns the last transition error for one key.
func (number *Number[Key]) Error(key Key) (error, bool) {
	stream, found := number.load(key)

	if !found {
		return nil, false
	}

	stream.mutex.RLock()
	err := stream.stream.Error()
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

// Reset replaces one key's committed state and clears its output and error.
func (number *Number[Key]) Reset(key Key, initial Frame) error {
	stream, err := number.stream(key)

	if err != nil {
		return err
	}

	stream.mutex.Lock()
	stream.stream.Reset(initial)
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
		state := stream.stream.Project()
		stream.mutex.RUnlock()

		return yield(key, state)
	})
}

/*
CrossSection evaluates one committed focal state against every committed peer,
folds pair outputs through reduce, then optionally finalizes the aggregate.
Candidate state from pair is observational and deliberately ignored.
*/
func (number *Number[Key]) CrossSection(
	key Key,
	pair Primitive,
	reduce Primitive,
	finalize Primitive,
) (Frame, bool, error) {
	focal, found := number.Project(key)

	if !found {
		return Frame{}, false, nil
	}

	accumulator := Frame{}
	output := Frame{}
	reduced := false
	var crossSectionErr error

	number.Range(func(peerKey Key, peer Frame) bool {
		if peerKey == key {
			return true
		}

		_, pairOutput, err := Step(pair, focal, peer)

		if err != nil {
			crossSectionErr = err
			return false
		}

		if !pairOutput.Finite() {
			crossSectionErr = errnie.Error(errnie.Err(
				errnie.Validation,
				"number pair output must be finite",
				nil,
			))
			return false
		}

		accumulator, output, err = Step(reduce, accumulator, pairOutput)

		if err != nil {
			crossSectionErr = err
			return false
		}

		reduced = true

		return true
	})

	if crossSectionErr != nil || !reduced || finalize == nil {
		return output, reduced, crossSectionErr
	}

	_, output, crossSectionErr = Step(finalize, accumulator, output)

	return output, true, crossSectionErr
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
		_, output, err := Step(score, Frame{}, state)

		if err != nil {
			selectionErr = err
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

	initial := Frame{}

	if number.initial != nil {
		initial = number.initial(key)
	}

	candidate := &numberStream{
		stream: Stream{
			primitive: number.primitive,
			state:     initial,
		},
	}
	stored, _ := number.streams.LoadOrStore(key, candidate)
	stream, valid := stored.(*numberStream)

	if !valid {
		return nil, primitiveError("number registry contains an invalid stream")
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
type Single func(input Frame) (Frame, error)

// NewSingle composes primitives into one state-carrying callable.
func NewSingle(primitives ...Primitive) Single {
	pipeline := Pipe(primitives...)
	state := Frame{}

	return func(input Frame) (Frame, error) {
		nextState, output, err := Step(pipeline, state, input)

		if err != nil {
			return Frame{}, err
		}

		state = nextState

		return output, nil
	}
}
