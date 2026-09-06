package correlation

import (
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Energies is the run of interval-normalized return energies r²/Δt: how much a
path moved per second of the time it took to move.

Dividing by the span is what makes two returns comparable when they were
realized over different stretches of time, which on an asynchronously
sampled path is the ordinary case rather than the exception.
*/
type Energies struct {
	core.PrimitiveError
	current core.Primitive
}

/*
NanosPerSecond is the nanosecond to second conversion the spans are
normalized by.
*/
const NanosPerSecond = 1e9

/*
NewEnergies configures the run held before anything has been shown.
*/
func NewEnergies(state core.Primitive) *Energies {
	return &Energies{
		current: state,
	}
}

/*
Next gathers the incoming returns and holds their energy rates.
*/
func (energies *Energies) Next(in core.Primitive) core.Primitive {
	gathered := core.Yield(
		core.From([]Interval(nil)), in,
		func(held, arriving []Interval) []Interval {
			return append(held, arriving...)
		},
	)

	energies.current = core.From(
		ratesOf(core.To[[]Interval](gathered)),
	)

	energies.current.Error(gathered.Error())

	return energies.current
}

/*
Read surfaces the energy rates for the boundary.
*/
func (energies *Energies) Read() any {
	return energies.current.Read()
}

/*
ratesOf normalizes each return's energy by the seconds it was realized over.
An interval that took no time carries no rate, since the quotient is not
defined there.
*/
func ratesOf(intervals []Interval) []float64 {
	rates := make([]float64, 0, len(intervals))

	for _, interval := range intervals {
		elapsed := float64(interval.To-interval.From) / NanosPerSecond

		if elapsed <= 0 {
			continue
		}

		rates = append(rates, interval.Value*interval.Value/elapsed)
	}

	return rates
}
