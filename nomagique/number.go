package nomagique

import "sync"

/*
Number is a state-carrying, self-adapting numeric unit, keyed by a comparable
stream identity (e.g. one per symbol). Writing

	number := nomagique.NewNumber[string](primitives...)
	output := number(symbol, input)

declares one living number that keeps an isolated window, baseline, and event
clock for every key it is fed.
*/
type Number[Key comparable] func(key Key, input Frame) (Frame, error)

/*
NewNumber composes primitives into one self-adapting numeric unit per key. The
per-key registry is a sync.Map (CAS-based, no mutex); each key gets its own
composed pipeline with its own state, so unrelated streams never smear each
other's windows.
*/
func NewNumber[Key comparable](primitives ...Primitive) Number[Key] {
	pipeline := Pipe(primitives...)

	var numbers sync.Map

	return func(key Key, input Frame) (Frame, error) {
		stored, found := numbers.Load(key)

		if !found {
			unit := NewSingle(pipeline)
			stored, _ = numbers.LoadOrStore(key, unit)
		}

		number := stored.(Single)

		return number(input)
	}
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
