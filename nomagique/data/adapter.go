package data

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Map carries one named payload. The flags say which payload it is.

Float maps publish numbers. Text maps publish text. Literal maps request
text. A map with none of those flags requests numbers.
*/
type Map[T any] struct {
	numeric bool
	textual bool
	literal bool
	Values  map[string]T
}

func NewMap(mapping ...string) Map[string] {
	mapper := Map[string]{Values: make(map[string]string)}

	for i := 0; i < len(mapping)-1; i += 2 {
		mapper.Values[mapping[i]] = mapping[i+1]
	}

	return mapper
}

func NewOutputMap() Map[float64] {
	return Map[float64]{
		numeric: true,
		Values:  make(map[string]float64),
	}
}

func NewTextMap() Map[string] {
	return Map[string]{
		textual: true,
		Values:  make(map[string]string),
	}
}

func NewLiteral(keys ...string) Map[string] {
	mapper := Map[string]{
		literal: true,
		Values:  make(map[string]string, len(keys)),
	}

	for _, key := range keys {
		mapper.Values[key] = key
	}

	return mapper
}

type State struct {
	input  Map[string]
	output Map[float64]
	text   Map[string]
}

func NewState(input Map[string], output ...Map[float64]) *State {
	out := NewOutputMap()

	if len(output) > 0 {
		out = output[0]
	}

	return &State{
		input:  input,
		output: out,
		text:   NewTextMap(),
	}
}

/*
Adapter binds a Measurement's domain keys to the native names used by
Primitives. String maps request native numbers. Literal maps request native
text. Float maps publish numbers. Text maps publish text. The State mapping
is the only place where native and domain names meet.
*/
type Adapter struct {
	*core.PrimitiveError
	measurement *Measurement
	state       *State
	values      Map[float64]
	literals    Map[string]
}

func NewAdapter(measurement *Measurement, state *State) *Adapter {
	return &Adapter{
		PrimitiveError: core.NewPrimitiveError(),
		measurement:    measurement,
		state:          state,
		values:         NewOutputMap(),
		literals:       NewTextMap(),
	}
}

func (wrapper *Adapter) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil || wrapper.state == nil {
				wrapper.Error(core.ErrShape)
				return
			}

			header := (*Map[string])(arriving)

			if header.textual {
				if wrapper.state.text.Values == nil {
					wrapper.Error(core.ErrShape)
					return
				}

				mapped := (*Map[string])(arriving)

				for nativeKey, value := range mapped.Values {
					domainKey := nativeKey

					if alias, held := wrapper.state.input.Values[nativeKey]; held {
						domainKey = alias
					}

					wrapper.state.text.Values[domainKey] = value
				}

				if !yield(unsafe.Pointer(wrapper)) {
					return
				}

				continue
			}

			if header.numeric {
				mapped := (*Map[float64])(arriving)

				for nativeKey, value := range mapped.Values {
					domainKey := nativeKey

					if alias, held := wrapper.state.input.Values[nativeKey]; held {
						domainKey = alias
					}

					wrapper.state.output.Values[domainKey] = value
				}

				if !yield(unsafe.Pointer(wrapper)) {
					return
				}

				continue
			}

			if header.literal {
				if wrapper.state.text.Values == nil {
					wrapper.Error(core.ErrShape)
					return
				}

				clear(wrapper.literals.Values)
				requested := (*Map[string])(arriving)

				for nativeKey := range requested.Values {
					domainKey := nativeKey

					if alias, held := wrapper.state.input.Values[nativeKey]; held {
						domainKey = alias
					}

					value, held := wrapper.state.text.Values[domainKey]

					if !held {
						wrapper.Error(core.ErrNotHeld)
						return
					}

					wrapper.literals.Values[nativeKey] = value
				}

				if !yield(unsafe.Pointer(&wrapper.literals)) {
					return
				}

				continue
			}

			clear(wrapper.values.Values)
			requested := (*Map[string])(arriving)

			for nativeKey := range requested.Values {
				domainKey := nativeKey

				if alias, held := wrapper.state.input.Values[nativeKey]; held {
					domainKey = alias
				}

				if value, held := wrapper.state.output.Values[domainKey]; held {
					wrapper.values.Values[nativeKey] = value
					continue
				}

				if wrapper.measurement == nil {
					wrapper.Error(core.ErrNotHeld)
					return
				}

				entry := Pull(wrapper.measurement.Read(domainKey))

				if entry == nil {
					wrapper.Error(core.ErrNotHeld)
					return
				}

				if entry.Err != nil {
					wrapper.Error(entry.Err)
					return
				}

				if entry.Metric == nil {
					wrapper.Error(core.ErrNotHeld)
					return
				}

				wrapper.values.Values[nativeKey] = entry.Metric.Raw
			}

			if !yield(unsafe.Pointer(&wrapper.values)) {
				return
			}
		}
	}
}
