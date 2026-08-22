package regulator

import "github.com/theapemachine/symm/types"

/*
markContext summarizes all executable position marks observed since the prior
account valuation. Dense marks condition the next control decision, but the
complete broker equity revision remains the supervised wallet outcome.
*/
type markContext struct {
	samples       int
	returnSamples int
	meanReturn    float64
	worstDrawdown float64
	minimumFloor  float64
	surgeFraction float64
}

type markAccumulator struct {
	markCount   int
	returnCount int
	returnSum   float64
	drawdown    float64
	floor       float64
	surgeCount  int
}

func (accumulator *markAccumulator) observe(
	feedback types.MarkFeedback,
	markReturn float64,
	hasPrevious bool,
) {
	accumulator.markCount++

	if hasPrevious {
		accumulator.returnSum += markReturn
		accumulator.returnCount++
	}

	if accumulator.markCount == 1 || feedback.PeakDrawdown < accumulator.drawdown {
		accumulator.drawdown = feedback.PeakDrawdown
	}

	if accumulator.markCount == 1 || feedback.FloorDistance < accumulator.floor {
		accumulator.floor = feedback.FloorDistance
	}

	if feedback.SurgeArmed {
		accumulator.surgeCount++
	}
}

func (accumulator *markAccumulator) snapshot() markContext {
	context := markContext{
		samples:       accumulator.markCount,
		returnSamples: accumulator.returnCount,
		worstDrawdown: accumulator.drawdown,
		minimumFloor:  accumulator.floor,
	}

	if accumulator.returnCount > 0 {
		context.meanReturn = accumulator.returnSum / float64(accumulator.returnCount)
	}

	if accumulator.markCount > 0 {
		context.surgeFraction = float64(accumulator.surgeCount) / float64(accumulator.markCount)
	}

	return context
}

func (accumulator *markAccumulator) reset() {
	accumulator.markCount = 0
	accumulator.returnCount = 0
	accumulator.returnSum = 0
	accumulator.drawdown = 0
	accumulator.floor = 0
	accumulator.surgeCount = 0
}

/*
hindsightContext summarizes all delayed opportunity and decision outcomes
evaluated by hindsight since the prior account valuation. It provides delayed
supervision without leaking future information into live decisions.
*/
type hindsightContext struct {
	samples              int
	capturedSamples      int
	missedSamples        int
	meanCapturedReturn   float64
	meanMissedReturn     float64
	falsePositiveReturn  float64
	thesisBlockCount     int
	confidenceBlockCount int
	supportBlockCount    int
	contradictBlockCount int
	graphBlockCount      int
}

func (context hindsightContext) opportunityPrevalence() float64 {
	if context.samples == 0 {
		return 0
	}

	return float64(context.capturedSamples+context.missedSamples) / float64(context.samples)
}

func (context hindsightContext) captureRatio() float64 {
	total := context.meanCapturedReturn*float64(context.capturedSamples) +
		context.meanMissedReturn*float64(context.missedSamples)

	if total <= 0 {
		return 0
	}

	return (context.meanCapturedReturn * float64(context.capturedSamples)) / total
}

type hindsightAccumulator struct {
	samples              int
	capturedCount        int
	missedCount          int
	capturedSum          float64
	missedSum            float64
	falsePositiveSum     float64
	thesisBlockCount     int
	confidenceBlockCount int
	supportBlockCount    int
	contradictBlockCount int
	graphBlockCount      int
}

func (accumulator *hindsightAccumulator) observe(feedback types.HindsightFeedback) {
	accumulator.samples++

	if feedback.Captured {
		accumulator.capturedCount++
		accumulator.capturedSum += feedback.RealizedReturn

		if feedback.RealizedReturn < 0 {
			accumulator.falsePositiveSum += -feedback.RealizedReturn
		}
	}

	if feedback.Missed {
		accumulator.missedCount++
		accumulator.missedSum += feedback.MissedReturn

		switch feedback.DominantBlocker {
		case "thesis_score":
			accumulator.thesisBlockCount++
		case "confidence":
			accumulator.confidenceBlockCount++
		case "support":
			accumulator.supportBlockCount++
		case "contradiction":
			accumulator.contradictBlockCount++
		case "graph":
			accumulator.graphBlockCount++
		}
	}
}

func (accumulator *hindsightAccumulator) snapshot() hindsightContext {
	context := hindsightContext{
		samples:              accumulator.samples,
		capturedSamples:      accumulator.capturedCount,
		missedSamples:        accumulator.missedCount,
		falsePositiveReturn:  accumulator.falsePositiveSum,
		thesisBlockCount:     accumulator.thesisBlockCount,
		confidenceBlockCount: accumulator.confidenceBlockCount,
		supportBlockCount:    accumulator.supportBlockCount,
		contradictBlockCount: accumulator.contradictBlockCount,
		graphBlockCount:      accumulator.graphBlockCount,
	}

	if accumulator.capturedCount > 0 {
		context.meanCapturedReturn = accumulator.capturedSum / float64(accumulator.capturedCount)
	}

	if accumulator.missedCount > 0 {
		context.meanMissedReturn = accumulator.missedSum / float64(accumulator.missedCount)
	}

	return context
}

func (accumulator *hindsightAccumulator) reset() {
	accumulator.samples = 0
	accumulator.capturedCount = 0
	accumulator.missedCount = 0
	accumulator.capturedSum = 0
	accumulator.missedSum = 0
	accumulator.falsePositiveSum = 0
	accumulator.thesisBlockCount = 0
	accumulator.confidenceBlockCount = 0
	accumulator.supportBlockCount = 0
	accumulator.contradictBlockCount = 0
	accumulator.graphBlockCount = 0
}

func regulatorContext(
	periodReturn float64,
	drawdown float64,
	active bool,
	marks markContext,
	hindsight hindsightContext,
) []float64 {
	activeVal := 0.0

	if active {
		activeVal = 1.0
	}

	return []float64{
		periodReturn,
		drawdown,
		activeVal,
		marks.meanReturn,
		marks.worstDrawdown,
		marks.minimumFloor,
		marks.surgeFraction,
		hindsight.opportunityPrevalence(),
		hindsight.meanCapturedReturn,
		hindsight.meanMissedReturn,
	}
}
