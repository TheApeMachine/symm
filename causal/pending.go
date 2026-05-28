package causal

import "time"

const (
	// causalForwardWindow is the horizon over which a flow/liquidity reading
	// is scored against the realized forward return it is meant to predict.
	causalForwardWindow = 30 * time.Second

	// causalPendingCap bounds the queue of unlabeled (awaiting-forward)
	// samples.
	causalPendingCap = 256
)

// pendingCausalSample is a feature reading whose forward-return label has not
// yet matured.
type pendingCausalSample struct {
	macroMomentum float64
	liquidity     float64
	localFlow     float64
	anchorPrice   float64
	openedAt      time.Time
}

// resolvePendingLocked promotes every pending feature reading whose forward
// window has elapsed into the training set, labeled with its realized forward
// return.
func (state *CausalSymbol) resolvePendingLocked(now time.Time) {
	if len(state.pendingSamples) == 0 || state.lastPrice <= 0 {
		return
	}

	kept := state.pendingSamples[:0]

	for _, pending := range state.pendingSamples {
		if now.Sub(pending.openedAt) < causalForwardWindow {
			kept = append(kept, pending)
			continue
		}

		if pending.anchorPrice <= 0 {
			continue
		}

		forwardReturn := (state.lastPrice - pending.anchorPrice) / pending.anchorPrice
		sample := newCausalSample(
			pending.macroMomentum, pending.liquidity, pending.localFlow, forwardReturn,
		)
		state.samples = append(state.samples, sample)

		if len(state.samples) > causalHistoryCap {
			state.samples = state.samples[len(state.samples)-causalHistoryCap:]
		}
	}

	state.pendingSamples = kept
}

// enqueuePendingLocked records the current feature reading to be labeled once
// causalForwardWindow has elapsed.
func (state *CausalSymbol) enqueuePendingLocked(
	macroMomentum, liquidity, localFlow, anchorPrice float64,
	now time.Time,
) {
	if anchorPrice <= 0 {
		return
	}

	state.pendingSamples = append(state.pendingSamples, pendingCausalSample{
		macroMomentum: macroMomentum,
		liquidity:     liquidity,
		localFlow:     localFlow,
		anchorPrice:   anchorPrice,
		openedAt:      now,
	})

	if len(state.pendingSamples) > causalPendingCap {
		state.pendingSamples = state.pendingSamples[len(state.pendingSamples)-causalPendingCap:]
	}
}
