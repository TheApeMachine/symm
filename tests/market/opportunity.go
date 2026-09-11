package market

import (
	"math"
	"time"
)

const (
	// These constants describe synthetic replay geometry, never strategy
	// policy: three observations form each visible leg and each future label;
	// the round base bid keeps assertions readable; the nonzero return size
	// separates positive and negative labels from floating-point noise.
	opportunityHorizonSteps  = 3
	opportunityStepsPerLeg   = 3
	opportunityBaseBid       = 100.0
	opportunityLogReturnSize = 0.02
)

// The repeating gaps are intentionally unequal fixture coordinates. Their
// magnitudes carry no market meaning; they only make duration an invalid proxy
// for the tape's exact ticker-ordinal horizon.
var opportunityEventGaps = [...]time.Duration{
	11 * time.Millisecond,
	1700 * time.Millisecond,
	43 * time.Millisecond,
	3100 * time.Millisecond,
	97 * time.Millisecond,
}

/*
OpportunityStep is one ticker-ordinal observation in an OpportunityTape.
Context is the signed fact available at EventTime. ExecutableBid is the price
at which the position could be sold at that same observation.
*/
type OpportunityStep struct {
	EventTime     time.Time
	ExecutableBid float64
	Context       float64
}

/*
OpportunityTape is a deterministic multi-leg ticker replay whose signed context
is aligned with the executable-bid return exactly HorizonSteps observations in
the future. The irregular event-time gaps make ticker ordinal, rather than an
elapsed-duration approximation, the only correct label clock.
*/
type OpportunityTape struct {
	Symbol       string
	HorizonSteps int
	Steps        []OpportunityStep
}

/*
NewOpportunityTape builds alternating positive and negative context legs. A
leg has three observations so both signs persist across multiple market events;
that is fixture shape, not a trading threshold. Three terminal observations
close the final three future-return labels.
*/
func NewOpportunityTape(symbol string, start time.Time, legs int) *OpportunityTape {
	observations := legs * opportunityStepsPerLeg
	steps := make([]OpportunityStep, observations+opportunityHorizonSteps)
	eventTime := start

	for index := range steps {
		eventTime = eventTime.Add(opportunityEventGaps[index%len(opportunityEventGaps)])
		context := 1.0

		if index/opportunityStepsPerLeg%2 != 0 {
			context = -1
		}

		steps[index] = OpportunityStep{
			EventTime: eventTime,
			Context:   context,
		}
	}

	for index := range steps {
		if index < opportunityHorizonSteps {
			steps[index].ExecutableBid = opportunityBaseBid

			continue
		}

		labelIndex := index - opportunityHorizonSteps
		steps[index].ExecutableBid = steps[labelIndex].ExecutableBid * math.Exp(
			opportunityLogReturnSize*steps[labelIndex].Context,
		)
	}

	return &OpportunityTape{
		Symbol:       symbol,
		HorizonSteps: opportunityHorizonSteps,
		Steps:        steps,
	}
}
