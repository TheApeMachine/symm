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
func (state *excitationState) decayTo(eventTimeSec float64, beta float64) {
	if !state.haveLast || eventTimeSec <= state.lastTimeSec {
		return
	}

	decayFactor := expNeg(beta, eventTimeSec-state.lastTimeSec)
	state.buySupport *= decayFactor
	state.sellSupport *= decayFactor
	state.lastTimeSec = eventTimeSec
}

/*
logLikelihoodSum accumulates log intensities across marked events strictly
after origin and at or before horizon.
*/
func (state *excitationState) logLikelihoodSum(
	marked []markedEvent,
	originSec, horizonSec float64,
	muBuy, muSell, alphaBB, alphaBS, alphaSB, alphaSS, beta float64,
) (float64, bool) {
	if len(marked) == 0 {
		return 0, false
	}

	state.lastTimeSec = marked[0].atSec
	state.haveLast = true
	logSum := 0.0

	for index := 0; index < len(marked); {
		eventTime := marked[index].atSec

		if eventTime > horizonSec {
			break
		}

		state.decayTo(eventTime, beta)

		end := index

		for end < len(marked) && marked[end].atSec == eventTime {
			end++
		}

		if eventTime > originSec {
			for _, event := range marked[index:end] {
				switch event.side {
				case sideBuy:
					lambda := muBuy + alphaBB*state.buySupport + alphaBS*state.sellSupport

					if lambda <= 0 {
						return 0, false
					}

					logSum += logPositive(lambda)
				case sideSell:
					lambda := muSell + alphaSB*state.buySupport + alphaSS*state.sellSupport

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
				state.buySupport += 1
			case sideSell:
				state.sellSupport += 1
			}
		}

		index = end
	}

	return logSum, true
}
