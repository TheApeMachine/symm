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

/*
Probe captures the carrier inline as it flows and passes it through unchanged.

Where Tap pulls a reading out of a node that has already computed it, Probe
observes the value moving between two stages of a Chain — the intermediate
result no accessor exposes because no node owns it. It is the functional
identity with a memory, so dropping one into a Chain cannot change what that
Chain computes.

Probe holds only the most recent value. To retain a window of them, place a
Store in a Split instead: a Store with no Reduce slot returns 0 and records
without disturbing the parallel sum.
*/
type Probe struct {
	Value Scalar
	Seen  bool
}

func (probe *Probe) Step(x Scalar) Scalar {
	probe.Value = x
	probe.Seen = true

	return x
}

var _ Node = (*Probe)(nil)
