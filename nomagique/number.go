package nomagique

import "sync"

/*
Number is a state-carrying, self-adapting numeric pipeline keyed by a
comparable stream identity. It owns each keyed Stream and exposes projections
so cross-sectional equations can compare committed numeric state without
reconstructing or copying histories in signal packages.

	number := nomagique.NewNumber[string](primitives...)
	output, err := number.Step(symbol, input)

Each key keeps an isolated window, baseline, and event clock.
*/
type Number[Key comparable] struct {
	primitive Primitive
	streams   sync.Map
}

/*
NewNumber composes primitives into one self-adapting numeric unit per key. The
per-key registry is a sync.Map (CAS-based, no mutex); each key gets its own
composed pipeline with its own state, so unrelated streams never smear each
other's windows.
*/
func NewNumber[Key comparable](primitives ...Primitive) *Number[Key] {
	return &Number[Key]{primitive: Pipe(primitives...)}
}

/*
Step applies one input to its keyed pipeline and commits the transition.
*/
func (number *Number[Key]) Step(key Key, input Frame) (Frame, error) {
	stream, err := number.stream(key)

	if err != nil {
		return Frame{}, err
	}

	return stream.Step(input)
}

/*
Project returns the committed state for one key.
*/
func (number *Number[Key]) Project(key Key) (Frame, bool) {
	stream, found := number.load(key)

	if !found {
		return Frame{}, false
	}

	return stream.Project(), true
}

/*
Range visits each committed keyed state. The state is copied by value and
cannot mutate the owning stream.
*/
func (number *Number[Key]) Range(yield func(Key, Frame) bool) {
	if number == nil || yield == nil {
		return
	}

	number.streams.Range(func(storedKey any, storedValue any) bool {
		key, validKey := storedKey.(Key)
		stream, validStream := storedValue.(*Stream)

		if !validKey || !validStream {
			return true
		}

		return yield(key, stream.Project())
	})
}

/*
CrossSection evaluates one committed focal state against every committed peer,
folds the pair outputs through a reducer, then applies a final projection. The
pair evaluator is observational: its candidate state is deliberately ignored
so only the Number's keyed streams own persistent path state.
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
ArgMax evaluates one score over every committed keyed state and returns the
unique maximum only when it exceeds the exact cross-sectional median. A tied
maximum has no unique owner and therefore produces no selection.
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

		if count >= len(values) {
			selectionErr = primitiveError("number cross-section exceeds Frame capacity")

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

	return (lower + upper) / 2
}

func selectValue(values []float64, target int) float64 {
	left := 0
	right := len(values) - 1

	for left < right {
		pivot := values[(left+right)/2]
		low := left
		high := right

		for low <= high {
			for values[low] < pivot {
				low++
			}

			for values[high] > pivot {
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

func (number *Number[Key]) stream(key Key) (*Stream, error) {
	if number == nil || number.primitive == nil {
		return nil, primitiveError("number primitive is nil")
	}

	if stream, found := number.load(key); found {
		return stream, nil
	}

	candidate := NewStream(number.primitive, Frame{})
	stored, _ := number.streams.LoadOrStore(key, candidate)
	stream, valid := stored.(*Stream)

	if !valid {
		return nil, primitiveError("number registry contains an invalid stream")
	}

	return stream, nil
}

func (number *Number[Key]) load(key Key) (*Stream, bool) {
	if number == nil {
		return nil, false
	}

	stored, found := number.streams.Load(key)

	if !found {
		return nil, false
	}

	stream, valid := stored.(*Stream)

	return stream, valid
}

/*
Single is the unkeyed numeric unit behind each per-key Number state.
*/
type Single func(input Frame) (Frame, error)

/*
NewSingle composes primitives into a single state-carrying callable. Its output
feeds the next primitive's input, and the composed state persists across calls
so windows and baselines accumulate history.

	unit := nomagique.NewSingle(temporal.A, statistic.B, probability.C)
	output := unit(input)
*/
func NewSingle(primitives ...Primitive) Single {
	pipeline := Pipe(primitives...)

	var (
		state Frame
	)

	return func(input Frame) (Frame, error) {
		nextState, output, err := Step(pipeline, state, input)

		if err != nil {
			return Frame{}, err
		}

		state = nextState

		return output, nil
	}
}
