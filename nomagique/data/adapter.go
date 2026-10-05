package data

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

type Map[T any] map[string]T

func NewMap(
	mapping ...string,
) Map[string] {
	mapper := make(Map[string])

	for i := 0; i < len(mapping)-1; i += 2 {
		mapper[mapping[i]] = mapping[i+1]
	}

	return mapper
}

func NewOutputMap(
	mapping ...any,
) Map[float64] {
	mapper := make(Map[float64])

	for i := 0; i < len(mapping)-1; i += 2 {
		if k, ok := mapping[i].(string); ok {
			if v, ok := mapping[i+1].(float64); ok {
				mapper[k] = v
			}
		}
	}

	return mapper
}

type State struct {
	input  Map[string]
	output Map[float64]
}

func NewState(
	input Map[string],
	output ...Map[float64],
) *State {
	out := make(Map[float64])
	if len(output) > 0 && output[0] != nil {
		out = output[0]
	}

	return &State{
		input:  input,
		output: out,
	}
}

/*
Adapter wraps a Measurement, and remaps its metric keys from the domain's
keys (provided by the System) to the keys used by a primitive internally.
*/
type Adapter struct {
	*core.PrimitiveError
	measurement *Measurement
	state       *State
}

func NewAdapter(
	measurement *Measurement, state *State,
) *Adapter {
	return &Adapter{
		PrimitiveError: core.NewPrimitiveError(),
		measurement:    measurement,
		state:          state,
	}
}

/*
Next checks if we're getting a Map[string] (reading) or a Map[float64] (writing).
*/
func (wrapper *Adapter) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			metrics := make([]Metric, 0, len(wrapper.state.output))

			for key, value := range wrapper.state.output {
				metrics = append(metrics, Metric{
					label: key,
					raw:   value,
				})
			}

			wrapper.measurement.Write(metrics...)

			if !yield(unsafe.Pointer(wrapper.measurement)) {
				return
			}

			return
		}

		for arriving := range in {
			switch m := (*(*any)(arriving)).(type) {
			case Map[string]:
				for _, domainKey := range m {
					entry := wrapper.measurement.Read(domainKey)
					if entry.Err != nil && wrapper.state != nil && wrapper.state.output != nil {
						if val, found := wrapper.state.output[domainKey]; found {
							entry = MetricEntry{
								Key: domainKey,
								Metric: Metric{
									label: domainKey,
									raw:   val,
								},
							}
						}
					}

					if !yield(unsafe.Pointer(&entry)) {
						return
					}
				}
			case Map[float64]:
				for domainKey, val := range m {
					if wrapper.state != nil && wrapper.state.output != nil {
						wrapper.state.output[domainKey] = val
					}
				}

				if !yield(arriving) {
					return
				}
			}
		}
	}
}
