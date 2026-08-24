package types

import (
	"fmt"
)

type bindingKind uint8

const (
	inputBinding bindingKind = iota + 1
	outputBinding
	stateBinding
)

/*
Binding connects an outer named fact to one local primitive port. Construct
bindings with In, Out, and State.
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

// State binds an outer state fact bidirectionally to a primitive-local port.
func State(fact Symbol, port Symbol) Binding {
	return Binding{kind: stateBinding, fact: fact, port: port}
}

/*
Wire gives one primitive a deliberately small local coordinate system. The
primitive receives only the input and state ports explicitly listed, and its
mapped outputs are projected back onto the incoming frame. No fallback, name
inference, or semantic coercion occurs.
*/
func Wire(primitive Primitive, bindings ...Binding) Primitive {
	program := append([]Binding(nil), bindings...)
	configurationError := validateBindings(program)

	return func(input Frame) Frame {
		if configurationError != nil {
			input.Err = configurationError

			return input
		}

		if primitive == nil {
			input.Err = PrimitiveError("wire primitive is nil")

			return input
		}

		local := Frame{}

		for _, binding := range program {
			switch binding.kind {
			case inputBinding:
				value, found := input.Get(binding.fact)

				if !found {
					input.Err = fmt.Errorf(
						"nomagique: wire input fact %s for port %s is missing",
						symbolLabel(binding.fact),
						symbolLabel(binding.port),
					)

					return input
				}

				local.Put(binding.port, value)
			case stateBinding:
				if value, found := input.Get(binding.fact); found {
					local.Put(binding.port, value)
				}
			}
		}

		result := Step(primitive, local)

		if result.Err != nil {
			input.Err = result.Err

			return input
		}

		for _, binding := range program {
			switch binding.kind {
			case stateBinding:
				if value, found := result.Get(binding.port); found {
					input.Put(binding.fact, value)
				} else {
					input.Delete(binding.fact)
				}
			case outputBinding:
				value, found := result.Get(binding.port)

				if !found {
					input.Err = fmt.Errorf(
						"nomagique: wire output port %s for fact %s is missing",
						symbolLabel(binding.port),
						symbolLabel(binding.fact),
					)

					return input
				}

				input.Put(binding.fact, value)
			}
		}

		return input
	}
}

func validateBindings(bindings []Binding) error {
	for index, binding := range bindings {
		if binding.kind < inputBinding || binding.kind > stateBinding {
			return PrimitiveError("wire contains an invalid binding")
		}

		for previous := range index {
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
					return PrimitiveError("wire state binding is ambiguous")
				}
			}
		}
	}

	return nil
}
