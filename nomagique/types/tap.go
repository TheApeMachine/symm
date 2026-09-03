package types

/*
Tap selects one reading from an upstream multi-output node and feeds it into
its own chain.

Section 8 resolves the diamond problem by letting an Equation expose
auxiliary readings as accessors alongside the single carrier its Step
returns. Tap is the composition-side half of that rule: it turns one of those
accessors back into a Node, so a downstream stage reads it as a slot instead
of the caller shuttling the value between stages.

Read names the reading. Into is the chain it feeds; when Into is present Tap
returns 0, so a Tap inside a Split records without disturbing the parallel
sum (Law of Sinks). With no Into, Tap simply emits the reading.

Degenerate behavior: an omitted Read has nothing to select and yields 0.
*/
type Tap struct {
	Read func() Scalar
	Into Node
}

func (tap *Tap) Step(Scalar) Scalar {
	if tap.Read == nil {
		return 0
	}

	value := tap.Read()

	if tap.Into == nil {
		return value
	}

	tap.Into.Step(value)

	return 0
}

var _ Node = (*Tap)(nil)
