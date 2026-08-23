package types

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
)

type bindingKind uint8

const (
	inputBinding bindingKind = iota + 1
	outputBinding
	stateBinding
)

/*
Binding connects an outer named fact to one local primitive port. Construct
bindings with In, Out, and State; the fields stay private so invalid directions
cannot be assembled accidentally.
*/
type Binding struct {
	kind bindingKind
	fact Symbol
	port Symbol
}

// In binds an outer input fact to a primitive-local input port.
func In(fact Symbol, port Symbol) Binding {
	return Binding{kind: inputBinding, fact: fact, port: port}
}

// Out projects a primitive-local output port back to an outer fact.
func Out(port Symbol, fact Symbol) Binding {
	return Binding{kind: outputBinding, fact: fact, port: port}
}

// State binds an outer state fact bidirectionally to a primitive-local state port.
func State(fact Symbol, port Symbol) Binding {
	return Binding{kind: stateBinding, fact: fact, port: port}
}

/*
Wire gives one primitive a deliberately small local coordinate system.

The outer Frame remains a collection of named facts and committed state. The
primitive receives only the input and state ports explicitly listed here. Its
mapped outputs are projected back onto the original outer input. No fallback,
name inference, or semantic coercion occurs.
*/
func Wire(primitive Primitive, bindings ...Binding) Primitive {
	program := append([]Binding(nil), bindings...)
	configurationError := validateBindings(program)

	return func(state Frame, input Frame) (Frame, Frame, error) {
		if configurationError != nil {
			return state, types.Frame{}, configurationError
		}

		if primitive == nil {
			return state, types.Frame{}, primitiveError("wire primitive is nil")
		}

		localInput := types.Frame{}
		localState := types.Frame{}

		for _, binding := range program {
			switch binding.kind {
			case inputBinding:
				value, found := input.Get(binding.fact)

				if !found {
					return state, types.Frame{}, fmt.Errorf(
						"nomagique: wire input fact %s for port %s is missing",
						symbolLabel(binding.fact),
						symbolLabel(binding.port),
					)
				}

				localInput.Put(binding.port, value)
			case stateBinding:
				if value, found := state.Get(binding.fact); found {
					localState.Put(binding.port, value)
				}
			}
		}

		candidateState, localOutput, err := Step(primitive, localState, localInput)

		if err != nil {
			return state, types.Frame{}, err
		}

		for port := range candidateState.All() {
			bound := false

			for _, binding := range program {
				if binding.kind == stateBinding && binding.port == port {
					bound = true
					break
				}
			}

			if !bound {
				return state, types.Frame{}, fmt.Errorf(
					"nomagique: wire primitive mutated unbound state port %s",
					symbolLabel(port),
				)
			}
		}

		nextState := state

		for _, binding := range program {
			if binding.kind != stateBinding {
				continue
			}

			if value, found := candidateState.Get(binding.port); found {
				nextState.Put(binding.fact, value)
			} else {
				nextState.Delete(binding.fact)
			}
		}

		output := input

		for _, binding := range program {
			if binding.kind != outputBinding {
				continue
			}

			value, found := localOutput.Get(binding.port)

			if !found {
				return state, types.Frame{}, fmt.Errorf(
					"nomagique: wire output port %s for fact %s is missing",
					symbolLabel(binding.port),
					symbolLabel(binding.fact),
				)
			}

			output.Put(binding.fact, value)
		}

		return nextState, output, nil
	}
}

func validateBindings(bindings []Binding) error {
	for index, binding := range bindings {
		if binding.kind < inputBinding || binding.kind > stateBinding {
			return primitiveError("wire contains an invalid binding")
		}

		for previous := 0; previous < index; previous++ {
			other := bindings[previous]

			switch binding.kind {
			case inputBinding:
				if other.kind == inputBinding && other.port == binding.port {
					return fmt.Errorf(
						"nomagique: wire input port %s is bound more than once",
						symbolLabel(binding.port),
					)
				}
			case outputBinding:
				if other.kind == outputBinding && other.fact == binding.fact {
					return fmt.Errorf(
						"nomagique: wire output fact %s is bound more than once",
						symbolLabel(binding.fact),
					)
				}
			case stateBinding:
				if other.kind == stateBinding &&
					(other.port == binding.port || other.fact == binding.fact) {
					return primitiveError("wire state binding is ambiguous")
				}
			}
		}
	}

	return nil
}
