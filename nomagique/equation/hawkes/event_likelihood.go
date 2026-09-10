package hawkes

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ScoredEvent is one event's log intensity on (origin,horizon].
*/
type ScoredEvent struct {
	Event
	LogIntensity float64
	Intensity    float64
}

/*
EventLikelihoodInput is the process window used to score events.
*/
type EventLikelihoodInput struct {
	Parameters
	Origin  float64
	Horizon float64
	Events  []Event
}

/*
EventLikelihood evaluates event log intensities on (origin,horizon].
*/
type EventLikelihood struct {
	core.Base[EventLikelihoodInput, ScoredEvent]
	intensity *Intensity
}

func NewEventLikelihood() *EventLikelihood {
	return &EventLikelihood{intensity: NewIntensity()}
}

func (op *EventLikelihood) Next(
	in iter.Seq[core.Primitive[EventLikelihoodInput, EventLikelihoodInput]],
) iter.Seq[core.Primitive[ScoredEvent, ScoredEvent]] {
	return func(yield func(core.Primitive[ScoredEvent, ScoredEvent]) bool) {
		for arriving := range in {
			input := arriving.Read()

			for _, event := range input.Events {
				if !(event.At > input.Origin && event.At <= input.Horizon) {
					continue
				}

				reading := IntensityResult{}

				for out := range op.intensity.Next(func(yield func(core.Primitive[IntensityInput, IntensityInput]) bool) {
					carrier := &core.Carrier[IntensityInput]{}
					yield(carrier.Carrier(IntensityInput{
						Parameters: input.Parameters,
						Side:       event.Side,
						Horizon:    event.At,
						Events:     input.Events,
					}))
				}) {
					reading = out.Read()
				}

				if !yield(op.Carrier(ScoredEvent{
					Event:        event,
					Intensity:    reading.Intensity,
					LogIntensity: math.Log(reading.Intensity),
				})) {
					return
				}
			}
		}
	}
}
