package types

import (
	"fmt"
	"sync"
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
wireFramePool recycles the local *Frame every Wire call needs. A Frame is
66KB+ (Data [MaxSlots]float64 alone), and local is passed by pointer through
primitive — an indirectly-called, dynamically-supplied func(*Frame) the
compiler cannot inline or prove non-escaping for, so its address unavoidably
forces local onto the heap on every call. Before the Primitive contract took
*Frame, this same local was passed by value and never escaped (Go copies a
value argument into the callee's own stack frame without needing the caller's
copy to outlive the call) — the value-based design was allocation-free by
accident of calling convention, not because nothing was being copied; it paid
a 66KB memcpy on every single Wire invocation instead. Pooling recovers the
zero-steady-state-allocation behavior under the pointer contract instead of
reintroducing that copy.
*/
var wireFramePool = sync.Pool{
	New: func() any { return &Frame{} },
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

	return func(input *Frame) {
		if configurationError != nil {
			input.Err = configurationError

			return
		}

		if primitive == nil {
			input.Err = PrimitiveError("wire primitive is nil")

			return
		}

		// The pool round-trip is isolated to this one Get/Put pair — no
		// early return between them — and the actual binding/Step logic
		// runs in a helper that reports its outcome through a plain return
		// value. A Put reachable from several different points inside the
		// loop below it (one per error branch) defeated escape analysis
		// hard enough that the *Frame from Get was heap-allocated fresh on
		// every single call, silently making the pool a no-op.
		local := wireFramePool.Get().(*Frame)
		*local = Frame{}

		runWire(primitive, program, input, local)

		wireFramePool.Put(local)
	}
}

/*
runWire executes one Wire invocation's binding/Step/projection sequence
against the caller's borrowed local frame, entirely in its own call frame with
a single normal return. Keeping this out of the closure that owns the pooled
Get/Put pair is what lets local's lifetime stay provably scoped to one call.

//go:noinline is deliberate, not a style choice: runWire is small enough that
the compiler inlines it into Wire's returned closure by default, and once
inlined, escape analysis re-evaluates local in the merged function — where it
sees Step(primitive, local) calling through the caller-supplied, opaque
primitive value and must assume the callee could retain the pointer. That
turned "leaking param: local" from a true-but-harmless fact about runWire's
own boundary into a real heap escape of the pool-borrowed Frame on every Wire
call, silently defeating wireFramePool. Blocking the inline keeps local's
escape analysis scoped to runWire's own call frame, where the same indirect
Step call does not force it onto the heap.
*/
//go:noinline
func runWire(primitive Primitive, program []Binding, input *Frame, local *Frame) {
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

				return
			}

			local.Put(binding.port, value)
		case stateBinding:
			if value, found := input.Get(binding.fact); found {
				local.Put(binding.port, value)
			}
		}
	}

	primitive(local)

	if local.Err != nil {
		input.Err = local.Err

		return
	}

	for _, binding := range program {
		switch binding.kind {
		case stateBinding:
			if value, found := local.Get(binding.port); found {
				input.Put(binding.fact, value)
			} else {
				input.Delete(binding.fact)
			}
		case outputBinding:
			value, found := local.Get(binding.port)

			if !found {
				input.Err = fmt.Errorf(
					"nomagique: wire output port %s for fact %s is missing",
					symbolLabel(binding.port),
					symbolLabel(binding.fact),
				)

				return
			}

			input.Put(binding.fact, value)
		}
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
