package correlation

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Scan is the correlation profile of two paths across every candidate offset:
what the relationship looks like when one path is moved in time against the
other.

The answer is the whole profile rather than the best offset in it, because a
maximum without the shape around it cannot be told apart from noise. Reducing
the profile to its peak, and reading how sharply it falls away, are separate
operations over what this yields.

Nothing about the grid is configured here either. The step is whatever the
paths' own sampling gave, and how far to look is whatever their retained
length supports, so a search never scans on an invented resolution.

The offset moves the incoming run against the counter run, which is what asks
whether one path led the other rather than merely moved with it.
*/
type Scan struct {
	core.PrimitiveError
	counter core.Primitive
	spacing core.Primitive
	span    core.Primitive
	current core.Primitive
}

/*
NewScan configures the run the incoming returns are scanned against, the
offset step in nanoseconds, and how many steps to take in each direction.
*/
func NewScan(counter, spacing, span core.Primitive) *Scan {
	return &Scan{
		counter: counter,
		spacing: spacing,
		span:    span,
	}
}

/*
Next gathers the incoming returns and holds the correlation at every
candidate offset, the contemporaneous estimate among them at zero.
*/
func (scan *Scan) Next(in core.Primitive) core.Primitive {
	gathered := core.Yield(
		core.From([]Interval(nil)), in,
		func(held, arriving []Interval) []Interval {
			return append(held, arriving...)
		},
	)

	counter := core.Yield(
		core.From([]Interval(nil)), scan.counter,
		func(held, arriving []Interval) []Interval {
			return append(held, arriving...)
		},
	)

	spacing := core.Yield(
		core.From(0.0), scan.spacing,
		func(held, value float64) float64 {
			return value
		},
	)

	span := core.Yield(
		core.From(0.0), scan.span,
		func(held, value float64) float64 {
			return value
		},
	)

	scan.current = core.From(scan.profile(
		gathered, counter,
		core.To[float64](spacing),
		int(core.To[float64](span)),
	))

	scan.current.Error(
		gathered.Error(), counter.Error(), spacing.Error(), span.Error(),
	)

	return scan.current
}

/*
Read surfaces the profile for the boundary.
*/
func (scan *Scan) Read() any {
	return scan.current.Read()
}

/*
profile estimates the correlation at each offset, pairing through a fresh
Overlap per candidate and dividing by the same two energies throughout:
shifting a run in time changes which returns coincided, never how much either
path moved.
*/
func (scan *Scan) profile(
	shifted, counter core.Primitive, spacing float64, span int,
) []Point {
	leftEnergy := NewEnergy(core.From(0.0)).Next(shifted)
	rightEnergy := NewEnergy(core.From(0.0)).Next(counter)

	profile := make([]Point, 0, 2*span+1)

	for step := -span; step <= span; step++ {
		offset := float64(step) * spacing

		products := NewOverlap(counter, core.From(offset)).Next(shifted)

		profile = append(profile, Point{
			X: offset,
			Y: core.To[float64](Correlation(products, leftEnergy, rightEnergy)),
		})
	}

	return profile
}
