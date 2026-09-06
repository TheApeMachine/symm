package core_test

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

/*
A wrong type is a composition error: two Primitives were joined that do not
speak about the same thing. Driving an operation over a carrier of the wrong
type asserts the failure travels with the value rather than being folded in
as a silent zero, which is the whole reason a Primitive carries its own error.
*/
func TestErrorTravels(t *testing.T) {
	tests.NewTestTable(
		tests.NewTestCase(
			"string", "hold", wrongType(),
			tests.WithGenerator[float64](0, 0, 5, false),
			tests.WithExpectedError[float64](core.ErrWrongType),
		),
	).Run(t)
}

/*
wrongType composes an operation over a carrier holding a string, which is the
mis-wiring the error exists to report.
*/
func wrongType() core.Primitive {
	return &mismatch{}
}

type mismatch struct {
	core.PrimitiveError
	current core.Primitive
}

func (primitive *mismatch) Next(in core.Primitive) core.Primitive {
	primitive.current = core.Yield(
		core.From(0.0), core.From("not a number"), func(held, value float64) float64 {
			return held + value
		},
	)

	return primitive.current
}

func (primitive *mismatch) Read() any {
	return primitive.current.Read()
}
