package store

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Retained holds the last value it was shown and hands it back whenever it is
asked, so a composition can refer to the same quantity more than once.

An operation's register normally advances the moment it is read, which means
a value can only be used where it is produced. Seeding an operation with a
Retained instead separates the two: the operation folds against what was
retained, and the retention updates from the result. That is what lets a
recurrence be written as a composition, since a recurrence is exactly an
expression that mentions its own previous value.
*/
type Retained struct {
	core.PrimitiveError
	held    core.Primitive
	current core.Primitive
}

/*
NewRetained configures the value retained before anything has been shown.
*/
func NewRetained(state core.Primitive) *Retained {
	return &Retained{
		current: state,
	}
}

/*
Next shadows what it currently holds before taking on what it is shown, so
the value from the step before is still there to be read. Whatever arrives is
an iterator, so the last of what it yields is what gets retained.

Offered nothing it yields the shadow, which is how a composition asks a
retention for the previous value rather than the present one.
*/
func (retained *Retained) Next(in core.Primitive) core.Primitive {
	if in == nil {
		return retained.held
	}

	retained.held = retained.current

	retained.current = core.Yield(
		retained.current, in, func(_, value float64) float64 {
			return value
		},
	)

	return retained.current
}

/*
Read surfaces the retained value for the boundary.
*/
func (retained *Retained) Read() any {
	return retained.current.Read()
}
