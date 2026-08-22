package regulator

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
