package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Absolute is the magnitude transfer |x|, discarding direction.
*/
type Absolute struct{}

func (Absolute) Step(x types.Number) types.Number {
	return types.Number(math.Abs(float64(x)))
}

/*
Negate is the additive inverse -x.
*/
type Negate struct{}

func (Negate) Step(x types.Number) types.Number {
	return -x
}

/*
Reciprocal is the multiplicative inverse 1/x.
Degenerate behavior: a zero input has no inverse and yields 0.
*/
type Reciprocal struct{}

func (Reciprocal) Step(x types.Number) types.Number {
	if x == 0 {
		return 0
	}

	return 1 / x
}

/*
Square is the second power x².
*/
type Square struct{}

func (Square) Step(x types.Number) types.Number {
	return x * x
}

/*
Sqrt is the principal square root.
Degenerate behavior: a negative input has no real root and yields 0.
*/
type Sqrt struct{}

func (Sqrt) Step(x types.Number) types.Number {
	if x < 0 {
		return 0
	}

	return types.Number(math.Sqrt(float64(x)))
}

/*
Log is the natural logarithm ln(x).
Degenerate behavior: a non-positive input is outside the domain and yields 0.
*/
type Log struct{}

func (Log) Step(x types.Number) types.Number {
	if x <= 0 {
		return 0
	}

	return types.Number(math.Log(float64(x)))
}

/*
Exp is the natural exponential e^x.
*/
type Exp struct{}

func (Exp) Step(x types.Number) types.Number {
	return types.Number(math.Exp(float64(x)))
}

/*
Atanh is the inverse hyperbolic tangent, mapping a correlation in (-1, 1)
onto the whole real line where its sampling dispersion is stationary.
Degenerate behavior: a saturated |x| >= 1 is outside the domain and yields 0.
*/
type Atanh struct{}

func (Atanh) Step(x types.Number) types.Number {
	if x <= -1 || x >= 1 {
		return 0
	}

	return types.Number(math.Atanh(float64(x)))
}

/*
Tanh is the hyperbolic tangent, the inverse of Atanh, mapping the real line
back into (-1, 1).
*/
type Tanh struct{}

func (Tanh) Step(x types.Number) types.Number {
	return types.Number(math.Tanh(float64(x)))
}

/*
Erfc is the complementary error function, the two-sided Gaussian tail mass
beyond x.
*/
type Erfc struct{}

func (Erfc) Step(x types.Number) types.Number {
	return types.Number(math.Erfc(float64(x)))
}

/*
Sign is the direction transfer, emitting -1, 0, or 1 and discarding magnitude.
*/
type Sign struct{}

func (Sign) Step(x types.Number) types.Number {
	if x > 0 {
		return 1
	}

	if x < 0 {
		return -1
	}

	return 0
}

/*
Scale multiplies the carrier by the value emitted from its Factor slot.
Degenerate behavior: an omitted Factor is the multiplicative identity, so the
carrier passes through unchanged.
*/
type Scale struct {
	Factor types.Node
}

func (scale *Scale) Step(x types.Number) types.Number {
	if scale.Factor == nil {
		return x
	}

	return x * scale.Factor.Step(x)
}

/*
Ratio divides the value emitted from its Numerator slot by the value emitted
from its Denominator slot, both evaluated on the same carrier.

Degenerate behavior: an omitted Numerator emits the carrier itself; an
omitted Denominator is the multiplicative identity; a zero denominator has no
quotient and yields 0.
*/
type Ratio struct {
	Numerator   types.Node
	Denominator types.Node
}

func (ratio *Ratio) Step(x types.Number) types.Number {
	numerator := x

	if ratio.Numerator != nil {
		numerator = ratio.Numerator.Step(x)
	}

	denominator := types.Number(1)

	if ratio.Denominator != nil {
		denominator = ratio.Denominator.Step(x)
	}

	if denominator == 0 {
		return 0
	}

	return numerator / denominator
}

/*
Difference subtracts the value emitted from its Subtrahend slot from the
value emitted from its Minuend slot, both evaluated on the same carrier.

Degenerate behavior: an omitted Minuend emits the carrier itself; an omitted
Subtrahend is the additive identity.
*/
type Difference struct {
	Minuend    types.Node
	Subtrahend types.Node
}

func (difference *Difference) Step(x types.Number) types.Number {
	minuend := x

	if difference.Minuend != nil {
		minuend = difference.Minuend.Step(x)
	}

	var subtrahend types.Number

	if difference.Subtrahend != nil {
		subtrahend = difference.Subtrahend.Step(x)
	}

	return minuend - subtrahend
}

/*
Constant emits a fixed value regardless of the carrier.

It exists so a composition can name a structural quantity — a unit
conversion, an algebraic identity element — as a node. It is NOT an escape
hatch for operational parameters: any window span, threshold, decay interval
or clamp bound must come from an adaptive engine, never from a Constant.
*/
type Constant struct {
	Value types.Number
}

func (constant Constant) Step(types.Number) types.Number {
	return constant.Value
}

/*
Floor bounds the carrier from below by the value emitted from its Bound slot.
Degenerate behavior: an omitted Bound imposes no floor.
*/
type Floor struct {
	Bound types.Node
}

func (floor *Floor) Step(x types.Number) types.Number {
	if floor.Bound == nil {
		return x
	}

	bound := floor.Bound.Step(x)

	if x < bound {
		return bound
	}

	return x
}

/*
Ceiling bounds the carrier from above by the value emitted from its Bound slot.
Degenerate behavior: an omitted Bound imposes no ceiling.
*/
type Ceiling struct {
	Bound types.Node
}

func (ceiling *Ceiling) Step(x types.Number) types.Number {
	if ceiling.Bound == nil {
		return x
	}

	bound := ceiling.Bound.Step(x)

	if x > bound {
		return bound
	}

	return x
}

/*
Finite passes finite carriers through and maps NaN and infinities to 0, so a
degenerate reading never propagates through a composition.
*/
type Finite struct{}

func (Finite) Step(x types.Number) types.Number {
	value := float64(x)

	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	return x
}

// Every operation satisfies the closed Node contract.
var (
	_ types.Node = Absolute{}
	_ types.Node = Negate{}
	_ types.Node = Reciprocal{}
	_ types.Node = Square{}
	_ types.Node = Sqrt{}
	_ types.Node = Log{}
	_ types.Node = Exp{}
	_ types.Node = Atanh{}
	_ types.Node = Tanh{}
	_ types.Node = Erfc{}
	_ types.Node = Sign{}
	_ types.Node = Constant{}
	_ types.Node = Finite{}
	_ types.Node = (*Scale)(nil)
	_ types.Node = (*Ratio)(nil)
	_ types.Node = (*Difference)(nil)
	_ types.Node = (*Floor)(nil)
	_ types.Node = (*Ceiling)(nil)
)

/*
UnitAxis squashes a signed deviation onto the open interval (0, 1), placing
zero at the midpoint.

Span is the node emitting how many deviations span the axis before the
squash saturates; beyond it values still order correctly, they simply crowd
toward the boundary they are heading for. It is a slot rather than a constant
because the scale over which a deviation is "large" is a property of the
data, not of the geometry.

Degenerate behavior: an omitted or non-positive Span is the identity scale.
*/
type UnitAxis struct {
	Span types.Node
}

func (axis *UnitAxis) Step(x types.Number) types.Number {
	span := types.Number(1)

	if axis.Span != nil {
		if emitted := axis.Span.Step(x); emitted > 0 {
			span = emitted
		}
	}

	return types.Number(0.5 + 0.5*math.Tanh(float64(x/span)))
}

var _ types.Node = (*UnitAxis)(nil)

/*
Accumulator is the unbounded running total of the value emitted from its
Source slot: every observation is added to the sum of every observation before
it, over the whole life of the node.

It is distinct from a Store, which retains a bounded adaptive window and
forgets what falls out of it. A running total has no window to forget from —
it is the quantity itself, carried forward, which is what a cumulative
measure such as executed volume or signed flow since an epoch actually is.

Total() reads the sum without advancing it, so a composition may branch on
the same accumulation from several slots.

Degenerate behavior: an omitted Source accumulates the carrier itself.
*/
type Accumulator struct {
	Source types.Node

	total types.Number
}

func (accumulator *Accumulator) Step(x types.Number) types.Number {
	value := x

	if accumulator.Source != nil {
		value = accumulator.Source.Step(x)
	}

	accumulator.total += value

	return accumulator.total
}

// Total returns the running sum without advancing it.
func (accumulator *Accumulator) Total() types.Number { return accumulator.total }

/*
Readings publishes the accumulation itself, so a terminal Projection harvests
it by walking the composition rather than being handed a pointer to it.
*/
func (accumulator *Accumulator) Readings() []types.Reading {
	return []types.Reading{{
		Label:     "total",
		Unit:      "count",
		Timescale: "instantaneous",
		Value:     accumulator.total,
		Defined:   true,
	}}
}

/*
Read exposes the running sum as a node that observes without advancing, so a
projection or any other reader can compose against the accumulation without
double-counting the observation that produced it.
*/
func (accumulator *Accumulator) Read() types.Node {
	return &reader{read: accumulator.Total}
}

/*
reader turns a value-returning method into a node that ignores the carrier.
It is how a stateful primitive offers itself to a composition for reading
rather than for advancing.
*/
type reader struct {
	read func() types.Number
}

func (node *reader) Step(types.Number) types.Number {
	if node.read == nil {
		return 0
	}

	return node.read()
}

var _ types.Node = (*reader)(nil)

var _ types.Node = (*Accumulator)(nil)

/*
Gate emits the value from its Source slot when its When slot emits a non-zero
value, and zero otherwise. It is how a composition states that a quantity only
counts under a condition — a buy-side notional, say — without the consumer
branching around the pipeline.

Degenerate behavior: an omitted Source emits the carrier; an omitted When
gates nothing open, so the node emits zero rather than silently passing the
value through unconditioned.
*/
type Gate struct {
	Source types.Node
	When   types.Node
}

func (gate *Gate) Step(x types.Number) types.Number {
	if gate.When == nil || gate.When.Step(x) == 0 {
		return 0
	}

	if gate.Source == nil {
		return x
	}

	return gate.Source.Step(x)
}

var _ types.Node = (*Gate)(nil)

// Slots exposes the nodes each operation is composed of, so a walk of the
// composition reaches everything nested inside them.
func (ratio *Ratio) Slots() []types.Node {
	return []types.Node{ratio.Numerator, ratio.Denominator}
}

func (difference *Difference) Slots() []types.Node {
	return []types.Node{difference.Minuend, difference.Subtrahend}
}

func (gate *Gate) Slots() []types.Node { return []types.Node{gate.Source, gate.When} }

func (accumulator *Accumulator) Slots() []types.Node { return []types.Node{accumulator.Source} }

func (scale *Scale) Slots() []types.Node { return []types.Node{scale.Factor} }
