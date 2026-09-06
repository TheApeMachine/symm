package tests

import (
	"errors"
	"fmt"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
folds are the reference each named operation is measured against, owned here
so that a test names an operation instead of restating it. A Primitive is
correct when it agrees with its fold across every generated value.

This is one source of truth, not two: a fold and a Primitive that are wrong
in the same way will agree. The folds are kept to expressions simple enough
to be read as obviously correct, and anything subtler than that needs a test
that checks a law rather than an example.
*/
var folds = map[string]func(register, in float64) float64{
	"hold":       func(register, _ float64) float64 { return register },
	"absolute":   func(_, in float64) float64 { return math.Abs(in) },
	"negate":     func(_, in float64) float64 { return -in },
	"square":     func(_, in float64) float64 { return in * in },
	"sign":       func(_, in float64) float64 { return sign(in) },
	"exp":        func(_, in float64) float64 { return math.Exp(in) },
	"tanh":       func(_, in float64) float64 { return math.Tanh(in) },
	"erfc":       func(_, in float64) float64 { return math.Erfc(in) },
	"reciprocal": func(held, in float64) float64 { return domained(held, in, in == 0, 1/in) },
	"sqrt":       func(held, in float64) float64 { return domained(held, in, in < 0, math.Sqrt(in)) },
	"log":        func(held, in float64) float64 { return domained(held, in, in <= 0, math.Log(in)) },
	"atanh":      func(held, in float64) float64 { return domained(held, in, in <= -1 || in >= 1, math.Atanh(in)) },
	"minimum":    func(held, in float64) float64 { return math.Min(held, in) },
	"maximum":    func(held, in float64) float64 { return math.Max(held, in) },

	"add":      func(register, in float64) float64 { return register + in },
	"subtract": func(register, in float64) float64 { return register - in },
	"multiply": func(register, in float64) float64 { return register * in },
	"divide":   func(register, in float64) float64 { return register / in },
}

/*
sign is the direction of a value, and is written out here rather than reached
for from math because Go has no sign function to reach for.
*/
func sign(value float64) float64 {
	if value > 0 {
		return 1
	}

	if value < 0 {
		return -1
	}

	return 0
}

/*
domained is the reference for a transform that is undefined somewhere: outside
its domain the register stands, because a value that cannot be transformed
must not silently become a zero.
*/
func domained(held, _ float64, outside bool, transformed float64) float64 {
	if outside {
		return held
	}

	return transformed
}

/*
TestCase drives one Primitive across a generated range and asserts every
reading against the operation it claims to perform. A case declares what is
under test and over what values; it never declares the answers, which is what
keeps a test file to a handful of lines no matter how many assertions it
produces.

The entity and operation strings do triple duty: the operation selects the
reference fold, and both appear in the Convey narration, so the test reads as
a specification rather than as a fixture.
*/
type TestCase[T core.Numeric] struct {
	entity    string
	operation string
	primitive core.Primitive
	seed      T
	from, to  T
	fuzz      bool
	expects   error
}

/*
poisons are the values that must never be quietly absorbed. A Primitive that
meets one has to pass it on: a NaN turned into a zero is a silent death that
surfaces much later as a wrong answer nobody can trace, whereas a NaN allowed
to travel announces the broken upstream that produced it.
*/
var poisons = []float64{math.NaN(), math.Inf(1), math.Inf(-1)}

/*
Option configures a TestCase at construction. Options keep a case to one call
with named intent rather than a struct literal whose fields have to be read
positionally.
*/
type Option[T core.Numeric] func(*TestCase[T])

/*
NewTestCase names the entity under test, the operation it performs, and the
Primitive itself. The operation must be one the harness knows a fold for,
since that fold is what the Primitive will be measured against.
*/
func NewTestCase[T core.Numeric](
	entity, operation string, primitive core.Primitive, options ...Option[T],
) *TestCase[T] {
	testCase := &TestCase[T]{entity: entity, operation: operation, primitive: primitive}

	for _, option := range options {
		option(testCase)
	}

	return testCase
}

/*
WithGenerator drives the primitive across a range, seeding its register.
Fuzzing additionally feeds each poison value, asserting the primitive carries
it through instead of absorbing it.
*/
func WithGenerator[T core.Numeric](seed, from, to T, fuzz bool) Option[T] {
	return func(testCase *TestCase[T]) {
		testCase.seed, testCase.from, testCase.to, testCase.fuzz = seed, from, to, fuzz
	}
}

/*
WithExpectedError declares that every step of this case must leave the
Primitive carrying the given error. It is how a case asserts that a failure
travels with a value instead of being swallowed, which is the whole point of
Primitives carrying their own errors.
*/
func WithExpectedError[T core.Numeric](expects error) Option[T] {
	return func(testCase *TestCase[T]) {
		testCase.expects = expects
	}
}

/*
Run drives the Primitive across the range and compares every reading to the
fold, then feeds the poison values if the case asks for them. A failure names
the operation and the value that broke it, so the narration is the diagnosis.
*/
func (testCase *TestCase[T]) Run(t *testing.T) {
	Convey(fmt.Sprintf("Given a %s as a Primitive", testCase.entity), t, func() {
		Convey(fmt.Sprintf("When I %s each generated value", testCase.operation), func() {
			fold := folds[testCase.operation]
			register := float64(testCase.seed)

			for in := testCase.from; in < testCase.to; in++ {
				register = fold(register, float64(in))

				out := testCase.primitive.Next(core.From(in))

				So(core.To[T](out), ShouldEqual, T(register))
				testCase.assertError(out)
			}
		})

		for _, poison := range testCase.poisons() {
			Convey(fmt.Sprintf("When I %s a poisoned %v", testCase.operation, poison), func() {
				reading := core.To[float64](testCase.primitive.Next(core.From(poison)))

				So(math.IsNaN(reading) || math.IsInf(reading, 0), ShouldBeTrue)
			})
		}
	})
}

/*
assertError checks what a reading is carrying against what the case expects.
A case that declares no error demands a clean one, so a Primitive that starts
failing silently is caught by every case in the suite rather than only by the
ones written to look for it.
*/
func (testCase *TestCase[T]) assertError(out core.Primitive) {
	if testCase.expects == nil {
		So(out.Error(), ShouldBeNil)

		return
	}

	So(errors.Is(out.Error(), testCase.expects), ShouldBeTrue)
}

/*
poisons yields the values a fuzzing case feeds, and an empty range otherwise,
so the caller loops over the result without needing to branch on it.
*/
func (testCase *TestCase[T]) poisons() []float64 {
	if !testCase.fuzz {
		return nil
	}

	return poisons
}

/*
TestTable groups the cases for one Primitive. It exists so a test file is a
single expression: the table names its cases, and running it is the whole
body of the test.
*/
type TestTable[T core.Numeric] struct {
	cases []*TestCase[T]
}

/*
NewTestTable collects the cases that exercise one Primitive.
*/
func NewTestTable[T core.Numeric](cases ...*TestCase[T]) *TestTable[T] {
	return &TestTable[T]{cases: cases}
}

/*
Run exercises every case in the table.
*/
func (test *TestTable[T]) Run(t *testing.T) {
	for _, testCase := range test.cases {
		testCase.Run(t)
	}
}
