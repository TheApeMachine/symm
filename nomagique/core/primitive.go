package core

import "fmt"

/*
Primitive is the entire contract of the system, and it is deliberately this
small. Everything is a Primitive: a number, an operation, a logic gate, a
store, a generated test input. Because the input and the output of Next are
both Primitives, anything composes with anything, and incompatible things can
travel together until the moment one of them actually has to understand
another.

Next advances the Primitive with an incoming value and yields a result. It is
a generator, not a function, and the caller has exactly one obligation: keep
calling until it returns nil. Yielding once and then nil denotes a single
value; yielding repeatedly denotes a sequence. Arity is therefore expressed
in time rather than in the signature, which is what allows one scalar, many
scalars, or a run of entirely unrelated Primitives to travel through the same
call without the type ever changing.

Nil belongs to the callee. It means only that this Primitive is done handing
things over for now, and carries no promise about what happens next: a
Primitive that wants to deliver in batches yields a nil to end the current
run and is fully alive for the following one. Whether a nil is final is the
Primitive's own business, and nothing may be inferred about it from outside.

A nil input means the caller has nothing to offer, and asks the Primitive to
answer on its own terms: an accumulator returns its seed so a fold has a base
to start from, while something that must be configured before use returns
whatever describes it. There is no single meaning fixed here, because the
right answer depends on what the caller needs at that point in a composition.

Read surfaces the underlying Go value. It exists so the boundary can leave
the algebra; inside a composition, Primitives hand each other Primitives.

Implementing this interface is the whole of joining the system. Do not add
methods to make a Primitive easier to use from one particular caller. If a
Primitive seems to need a helper, it is doing more than one thing and should
be decomposed until it does not.
*/
type Primitive interface {
	Next(Primitive) Primitive
	Read() any
	Error(...error) error
}

/*
Proto carries a Go value into the algebra. It is what From wraps a value in
so that a number, a string, or anything else can travel as a Primitive, and
it does nothing beyond holding what it was given.
*/
type Proto struct {
	PrimitiveError
	state     any
	caller    Primitive
	delivered bool
}

/*
NewProto wraps a value so it can travel as a Primitive. Configuration belongs
in a constructor rather than in Next, so that a Primitive is fully formed
before it is ever stepped, and stepping stays a pure advance.
*/
func NewProto(state any) *Proto {
	return &Proto{
		state: state,
	}
}

/*
Next hands back the value this Proto holds, and then ends the run for
whoever asked. A carrier has nothing to advance, so it yields itself
whatever it is stepped with, and answers the same caller's second call with
nil.

The nil is what makes a carrier drainable. A consumer keeps calling until it
is told there is nothing more, so a carrier that always answered with itself
would never let a fold finish. The run belongs to the caller rather than to
the carrier: a different caller starts its own run and is handed the value
again, which is what lets one carrier feed several stages of a composition
and lets a stage step the same carrier on the tick after next.
*/
func (primitive *Proto) Next(in Primitive) Primitive {
	if primitive.delivered && in == primitive.caller {
		primitive.delivered = false

		return nil
	}

	primitive.caller, primitive.delivered = in, true

	return primitive
}

/*
Read surfaces the wrapped value for the boundary.
*/
func (primitive *Proto) Read() any {
	return primitive.state
}

/*
From lifts a Go value into the algebra, and is the only way in. A float64
cannot satisfy Primitive on its own, so something has to hold it; From is
that something, and keeping it generic means the same door admits numbers,
strings, maps, or anything a later domain invents.
*/
func From[T any](value T) Primitive {
	return NewProto(value)
}

/*
To reads a Go value back out, and is the only way out. It is the counterpart
to From and belongs at the boundary of a composition, not inside one: a
Primitive that reaches for To in order to understand its neighbour has
usually been given a neighbour it should not have been composed with.

A Primitive that holds nothing, or holds something of another type, reads as
the zero value rather than panicking, because a composition is expected to be
assembled correctly by whoever assembled it.
*/
func To[T any](primitive Primitive) T {
	var (
		zero    T
		readout any
	)

	if primitive == nil {
		return zero
	}

	if readout = primitive.Read(); readout == nil {
		return zero
	}

	value, ok := readout.(T)

	if !ok {
		primitive.Error(ErrConversion)
	}

	return value
}

/*
Yield drains a Primitive, folding everything it hands over into an
accumulated value. The contract says a caller keeps calling until nil, so
anything consuming a Primitive goes through here rather than reading once and
silently dropping whatever else was on offer. Whatever is being drained
decides how much that is: one value, a batch, or a run of unrelated things.

Offered nothing to drain, it hands back the accumulator untouched. That is
what lets a composition begin anywhere: the first step of a pipeline has no
upstream, so its seed becomes the base the rest of the fold builds on, and no
Primitive has to guard for it.

Failures travel with the value. Whatever it steps hands its errors to the
result, and a value of the wrong type is recorded rather than silently folded
in as a zero, so a mis-wired composition is visible at the boundary instead of
producing a confidently wrong number.

Whatever arrives is an iterator, so it is stepped until it says it has
nothing more. One value or many, the fold sees each of them, and no Primitive
has to know which of the two it was handed.
*/
func Yield[T any](left, right Primitive, fold func(held, in T) T) Primitive {
	if right == nil {
		return left
	}

	out := NewProto(nil)
	out.Error(right.Error())

	held := To[T](left)

	for value := right.Next(left); value != nil; value = right.Next(left) {
		out.Error(value.Error())

		typed, ok := value.Read().(T)

		if !ok {
			out.Error(fmt.Errorf(
				"%w: %T is not %T", ErrWrongType, value.Read(), held,
			))

			continue
		}

		held = fold(held, typed)
	}

	out.state = held
	return out
}

/*
Numeric constrains the values that can be read back as numbers. It exists so
generic code can be written against the arithmetic Primitives without giving
up compile-time types, and should grow only when a genuinely new numeric
representation enters the system.
*/
type Numeric interface {
	~float64 | ~float32 | ~int | ~int64
}
