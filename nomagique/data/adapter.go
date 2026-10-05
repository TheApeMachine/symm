package data

import (
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

type Map[T any] struct {
	numeric bool
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

type State struct {
	input  Map[string]
	output Map[float64]
}

func NewState(input Map[string], output ...Map[float64]) *State {
	out := NewOutputMap()

	if len(output) > 0 {
		out = output[0]
	}

	return &State{input: input, output: out}
}

/*
Adapter binds a Measurement's domain keys to the native names used by
Primitives. String maps request native inputs; float maps publish native
outputs. The State mapping is the only place where native and domain names
meet.
*/
type Adapter struct {
	*core.PrimitiveError
	measurement *Measurement
	state       *State
	values      Map[float64]
}

func NewAdapter(measurement *Measurement, state *State) *Adapter {
	return &Adapter{
		PrimitiveError: core.NewPrimitiveError(),
		measurement:    measurement,
		state:          state,
		values:         NewOutputMap(),
	}
}

func (wrapper *Adapter) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil || wrapper.state == nil {
				wrapper.Error(core.ErrShape)
				return
			}

			if (*Map[string])(arriving).numeric {
				mapped := (*Map[float64])(arriving)

				for nativeKey, value := range mapped.Values {
					domainKey := nativeKey

					if alias, ok := wrapper.state.input.Values[nativeKey]; ok {
						domainKey = alias
					}

					wrapper.state.output.Values[domainKey] = value
				}

				if !yield(unsafe.Pointer(wrapper)) {
					return
				}

				continue
			}

			clear(wrapper.values.Values)
			requested := (*Map[string])(arriving)

			for nativeKey := range requested.Values {
				domainKey := nativeKey

				if alias, ok := wrapper.state.input.Values[nativeKey]; ok {
					domainKey = alias
				}

				if value, ok := wrapper.state.output.Values[domainKey]; ok {
					wrapper.values.Values[nativeKey] = value
					continue
				}

				if wrapper.measurement == nil {
					wrapper.Error(core.ErrNotHeld)
					return
				}

				if domainKey == "At" {
					wrapper.values.Values[nativeKey] = float64(wrapper.measurement.At.UnixNano()) / float64(time.Second)
					continue
				}

				if domainKey == "From" {
					wrapper.values.Values[nativeKey] = float64(wrapper.measurement.From.UnixNano()) / float64(time.Second)
					continue
				}

				if domainKey == "SeqIdx" {
					wrapper.values.Values[nativeKey] = float64(wrapper.measurement.SeqIdx)
					continue
				}

				entry := wrapper.measurement.Read(domainKey)

				if entry.Err != nil {
					continue
				}

				wrapper.values.Values[nativeKey] = entry.Metric.Raw
			}

			if !yield(unsafe.Pointer(&wrapper.values)) {
				return
			}
		}
	}
}
