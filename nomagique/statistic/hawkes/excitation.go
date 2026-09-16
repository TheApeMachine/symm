package hawkes

/*
excitationState tracks running Hawkes excitation sums while walking marked
events in chronological order.
*/
type excitationState struct {
	buySupport  float64
	sellSupport float64
	lastTimeSec float64
	haveLast    bool
}

/*
decayTo advances excitation sums to eventTimeSec under exponential decay.
*/
func (excitationState *excitationState) decayTo(eventTimeSec float64, beta float64) {
	if !excitationState.haveLast || eventTimeSec <= excitationState.lastTimeSec {
		return
	}

	decayFactor := expNeg(beta, eventTimeSec-excitationState.lastTimeSec)
	excitationState.buySupport *= decayFactor
	excitationState.sellSupport *= decayFactor
	excitationState.lastTimeSec = eventTimeSec
}

/*
logLikelihoodSum accumulates log intensities across marked events strictly
after origin and at or before horizon.
*/
func (excitationState *excitationState) logLikelihoodSum(
	marked []markedEvent,
	originSec, horizonSec float64,
	muBuy, muSell, alphaBB, alphaBS, alphaSB, alphaSS, beta float64,
) (float64, bool) {
	if len(marked) == 0 {
		return 0, false
	}

	excitationState.lastTimeSec = marked[0].atSec
	excitationState.haveLast = true
	logSum := 0.0

	for index := 0; index < len(marked); {
		eventTime := marked[index].atSec

		if eventTime > horizonSec {
			break
		}

		excitationState.decayTo(eventTime, beta)

		end := index

		for end < len(marked) && marked[end].atSec == eventTime {
			end++
		}

		if eventTime > originSec {
			for _, event := range marked[index:end] {
				switch event.side {
				case sideBuy:
					lambda := muBuy + alphaBB*excitationState.buySupport + alphaBS*excitationState.sellSupport

					if lambda <= 0 {
						return 0, false
					}

					logSum += logPositive(lambda)
				case sideSell:
					lambda := muSell + alphaSB*excitationState.buySupport + alphaSS*excitationState.sellSupport

					if lambda <= 0 {
						return 0, false
					}

					logSum += logPositive(lambda)
				}
			}
		}

		for _, event := range marked[index:end] {
			switch event.side {
			case sideBuy:
				excitationState.buySupport += 1
			case sideSell:
				excitationState.sellSupport += 1
			}
		}

		index = end
	}

	return logSum, true
}
