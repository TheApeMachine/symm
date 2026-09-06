package strategy

import (
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
	"time"
)

/* learningMarket owns persistent wallets and the latest ordered impulse. */
type learningMarket struct {
	symbol, status     string
	regions            []learning.Region
	sequence           []uint64
	conditions         []uint64
	authority          float64
	opportunityHorizon time.Duration
	lanes              []learningLane
	context            []uint64
	actions            []LearningAction
	events             []hindsight.LearningEvent
	at                 time.Time
	seq                hindsight.CaptureSequence
	capture            hindsight.CaptureIdentity
	gridVersion        uint64

	/*
		epochs measures this instrument's own cadence of impulse change: the
		mean interval between grid versions that actually moved. The decision
		horizon is derived from it, so a fast instrument is scored over a fast
		window and a slow one is not judged on noise.
	*/
	epochAt   time.Time
	epochMean float64
	epochs    uint64

	// exposure is the policy lane's inventory history, used to judge episodes
	// the delay line confirms after the fact.
	exposure []exposureSpan
}

/* epoch folds one observed interval between impulse changes into the mean. */
func (market *learningMarket) epoch(at time.Time) {
	if !market.epochAt.IsZero() && at.After(market.epochAt) {
		market.epochs++
		market.epochMean += (at.Sub(market.epochAt).Seconds() - market.epochMean) / float64(market.epochs)
	}

	market.epochAt = at
}

/*
horizon is the measured forward window every decision in this market is scored
over. It is unavailable until an interval has actually been observed; an
unmeasured horizon resolves nothing rather than inventing a default one.
*/
func (market *learningMarket) horizon() time.Duration {
	if market.opportunityHorizon > 0 {
		return market.opportunityHorizon
	}

	if market.epochs == 0 || market.epochMean <= 0 {
		return 0
	}

	return time.Duration(market.epochMean * horizonEpochs * float64(time.Second))
}
