package strategy

import (
	"slices"
	"time"

	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
)

/*
FrameDelimiter is the reserved delimiter token marking temporal state transitions
in precursor context paths. Bit 53 is distinct from ConditionToken (bit 52) and
quantity identities (bits 4..51).
*/
const FrameDelimiter uint64 = 1 << 53

/*
learningMarket owns independent per-symbol virtual wallets and the evolving
temporal precursor history across distinct Impulse state changes.
*/
type learningMarket struct {
	symbol            string
	status            string
	regions           []learning.Region
	currentConditions []uint64
	history           [][]uint64
	authority         float64
	lanes             []learningLane
	context           []uint64
	actions           []LearningAction
	events            []hindsight.LearningEvent
	at                time.Time
	seq               hindsight.CaptureSequence
	capture           hindsight.CaptureIdentity
	gridVersion       uint64

	// exposure is the policy lane's inventory history, used to judge episodes
	// the delay line confirms after the fact.
	exposure []exposureSpan
}

/*
AdvanceImpulse updates the market's observation state and advances the temporal
precursor history only when the ordered active regions actually change.
*/
func (market *learningMarket) AdvanceImpulse(regions []learning.Region) bool {
	conditions := make([]uint64, len(regions))

	for index, region := range regions {
		conditions[index] = region.Condition
	}

	if slices.Equal(conditions, market.currentConditions) {
		return false
	}

	if len(market.currentConditions) > 0 {
		market.history = append(market.history, append([]uint64(nil), market.currentConditions...))
	}

	market.currentConditions = conditions
	market.regions = append(market.regions[:0], regions...)

	market.authority = 0
	strength := 0.0

	for _, region := range regions {
		market.authority += region.Strength * region.Authority
		strength += region.Strength
	}

	if strength > 0 {
		market.authority /= strength
	}

	return true
}

/*
PrecursorContext arranges precursor history so that shorter prefixes correspond
to shorter recent precursor histories: current impulse first (in strength order),
followed by FRAME and previous impulses in reverse chronological order.
*/
func (market *learningMarket) PrecursorContext() []uint64 {
	if len(market.currentConditions) == 0 {
		return nil
	}

	size := len(market.currentConditions)

	for _, past := range market.history {
		size += 1 + len(past)
	}

	context := make([]uint64, 0, size)
	context = append(context, market.currentConditions...)

	for index := len(market.history) - 1; index >= 0; index-- {
		context = append(context, FrameDelimiter)
		context = append(context, market.history[index]...)
	}

	return context
}
