package system

import "math"

/*
LeadSpan is how many ticks or trade indices to pull back from B and from C,
given the precursor length A→B. ExecutionLeadFraction is applied to that
span so entry and exit can fill before the move they target.
*/
func (detector *Detector) LeadSpan(precursorSpan int64) int64 {
	if detector == nil || precursorSpan < 1 {
		return 0
	}

	lead := int64(math.Floor(detector.ExecutionLeadFraction * float64(precursorSpan)))

	if lead >= precursorSpan {
		return precursorSpan - 1
	}

	if lead < 0 {
		return 0
	}

	return lead
}

/*
ApplyExecutionLead shifts B and C earlier by one lead, taken from the A→B
tick span. Index and tick move by that same count so the pair still names
one print. Entry is the shifted B and exit is the shifted C.
*/
func (detector *Detector) ApplyExecutionLead(
	aIdx, aTick, bIdx, bTick, cIdx, cTick int64,
) (outBIdx, outBTick, outCIdx, outCTick, entryIdx, entryTick, exitIdx, exitTick int64) {
	precursorTick := bTick - aTick
	if precursorTick < 1 {
		precursorTick = 1
	}

	lead := detector.LeadSpan(precursorTick)

	for lead > 0 && (bIdx-lead < aIdx || bTick-lead < aTick) {
		lead--
	}

	outBIdx = bIdx - lead
	outBTick = bTick - lead
	outCIdx = cIdx - lead
	outCTick = cTick - lead

	entryIdx, entryTick = outBIdx, outBTick
	exitIdx, exitTick = outCIdx, outCTick

	return outBIdx, outBTick, outCIdx, outCTick, entryIdx, entryTick, exitIdx, exitTick
}
