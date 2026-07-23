package correlation

import (
	"math"
)

/*
pairRevision is one peer-side covariance and cohort-sum update staged around a
variance change so old and new correlations use the correct denominators.
*/
type pairRevision struct {
	peerSymbol string
	peer       *symbolState
	delta      float64
	oldRho     float64
	oldOK      bool
	seed       bool
}

/*
pair returns the unordered covariance cell for two symbols without allocating
a composite string key.
*/
func (section *Section) pair(left, right string) *pairState {
	first, second := orderPair(left, right)
	peers := section.pairs[first]

	if peers == nil {
		return nil
	}

	return peers[second]
}

/*
storePair writes the unordered covariance cell for two symbols.
*/
func (section *Section) storePair(left, right string, pair *pairState) {
	first, second := orderPair(left, right)
	peers := section.pairs[first]

	if peers == nil {
		peers = make(map[string]*pairState)
		section.pairs[first] = peers
	}

	peers[second] = pair
}

/*
orderPair returns lexicographic endpoints so each unordered pair has one home.
*/
func orderPair(left, right string) (string, string) {
	if left > right {
		return right, left
	}

	return left, right
}

/*
applyRightEdge folds the newest return interval into variance and every peer
pair, creating a seeded pair when both sides first become ready.
*/
func (section *Section) applyRightEdge(symbol string, state *symbolState) {
	last := len(state.samples) - 1
	previous := state.samples[last-1]
	current := state.samples[last]

	if !hayashiIntervalOK(previous, current) {
		return
	}

	ret := state.logPrices[last] - state.logPrices[last-1]
	pending := section.revisionBuf[:0]

	if cap(pending) < len(section.symbols) {
		pending = make([]pairRevision, 0, len(section.symbols))
	}

	for peerSymbol, peer := range section.symbols {
		if peerSymbol == symbol || len(peer.samples) < 2 {
			continue
		}

		existing := section.pair(symbol, peerSymbol)

		if existing == nil {
			pending = append(pending, pairRevision{
				peerSymbol: peerSymbol,
				peer:       peer,
				seed:       true,
			})

			continue
		}

		oldRho, oldOK := pairCorrelation(existing, state.variance, peer.variance)
		pending = append(pending, pairRevision{
			peerSymbol: peerSymbol,
			peer:       peer,
			delta: overlapCovarianceRecent(
				ret, previous.At, current.At, peer.samples, peer.logPrices,
			),
			oldRho: oldRho,
			oldOK:  oldOK,
		})
	}

	state.variance += ret * ret
	section.revisionBuf = pending
	section.commitRevisions(symbol, state, pending)
}

/*
dropLeftEdge removes the oldest sample and subtracts the retired interval from
streaming Hayashi state. After eviction it re-derives variance from log-prices
so IEEE drift cannot accumulate across millions of ± ret² updates.
*/
func (section *Section) dropLeftEdge(symbol string, state *symbolState) {
	if len(state.samples) < 2 {
		state.samples = state.samples[:0]
		state.logPrices = state.logPrices[:0]
		state.variance = 0

		return
	}

	previous := state.samples[0]
	current := state.samples[1]

	if hayashiIntervalOK(previous, current) {
		ret := state.logPrices[1] - state.logPrices[0]
		pending := section.revisionBuf[:0]

		if cap(pending) < len(section.symbols) {
			pending = make([]pairRevision, 0, len(section.symbols))
		}

		for peerSymbol, peer := range section.symbols {
			if peerSymbol == symbol || len(peer.samples) < 2 {
				continue
			}

			existing := section.pair(symbol, peerSymbol)

			if existing == nil {
				continue
			}

			oldRho, oldOK := pairCorrelation(existing, state.variance, peer.variance)
			pending = append(pending, pairRevision{
				peerSymbol: peerSymbol,
				peer:       peer,
				delta: -overlapCovarianceRetired(
					ret, previous.At, current.At, peer.samples, peer.logPrices,
				),
				oldRho: oldRho,
				oldOK:  oldOK,
			})
		}

		state.variance -= ret * ret

		if state.variance < 0 {
			state.variance = 0
		}

		section.revisionBuf = pending
		section.commitRevisions(symbol, state, pending)
	}

	state.samples = state.samples[1:]
	state.logPrices = state.logPrices[1:]
	section.resyncVariance(symbol, state)
}

/*
exactVariance rebuilds Hayashi variance from retained log-prices so streaming
± ret² updates cannot leave residual float noise in the denominator.
*/
func exactVariance(state *symbolState) float64 {
	if len(state.samples) < 2 || len(state.logPrices) != len(state.samples) {
		return 0
	}

	sum := 0.0

	for index := 0; index < len(state.logPrices)-1; index++ {
		if !hayashiIntervalOK(state.samples[index], state.samples[index+1]) {
			continue
		}

		ret := state.logPrices[index+1] - state.logPrices[index]
		sum += ret * ret
	}

	return sum
}

/*
resyncVariance replaces streaming variance with the exact retained sum and
refreshes cohort aggregates when the denominator moved.
*/
func (section *Section) resyncVariance(symbol string, state *symbolState) {
	exact := exactVariance(state)

	if exact == state.variance {
		return
	}

	previousVariance := state.variance

	for peerSymbol, peer := range section.symbols {
		if peerSymbol == symbol {
			continue
		}

		existing := section.pair(symbol, peerSymbol)

		if existing == nil {
			continue
		}

		oldRho, oldOK := pairCorrelation(existing, previousVariance, peer.variance)
		newRho, newOK := pairCorrelation(existing, exact, peer.variance)
		section.shiftCohort(state, oldRho, oldOK, newRho, newOK)
		section.shiftCohort(peer, oldRho, oldOK, newRho, newOK)
	}

	state.variance = exact
}

/*
commitRevisions applies seeded pairs or covariance deltas after variance has
already moved to its post-interval value.
*/
func (section *Section) commitRevisions(
	symbol string,
	state *symbolState,
	pending []pairRevision,
) {
	for _, revision := range pending {
		if revision.seed {
			section.seedPair(symbol, state, revision.peerSymbol, revision.peer)

			continue
		}

		existing := section.pair(symbol, revision.peerSymbol)

		if existing == nil {
			continue
		}

		existing.covariance += revision.delta
		newRho, newOK := pairCorrelation(
			existing, state.variance, revision.peer.variance,
		)
		section.shiftCohort(state, revision.oldRho, revision.oldOK, newRho, newOK)
		section.shiftCohort(
			revision.peer, revision.oldRho, revision.oldOK, newRho, newOK,
		)
	}
}

/*
seedPair materializes one pair from a full Hayashi covariance pass so later
ticks can update that pair incrementally.
*/
func (section *Section) seedPair(
	leftSymbol string,
	left *symbolState,
	rightSymbol string,
	right *symbolState,
) {
	if section.pair(leftSymbol, rightSymbol) != nil {
		return
	}

	pair := &pairState{
		covariance: hayashiCovariance(
			left.samples, right.samples, left.logPrices, right.logPrices,
		),
	}
	section.storePair(leftSymbol, rightSymbol, pair)
	rho, ok := pairCorrelation(pair, left.variance, right.variance)
	section.shiftCohort(left, 0, false, rho, ok)
	section.shiftCohort(right, 0, false, rho, ok)
}

/*
shiftCohort updates one symbol's running mean inputs when a peer correlation
appears, disappears, or changes value.
*/
func (section *Section) shiftCohort(
	state *symbolState,
	oldRho float64,
	oldOK bool,
	newRho float64,
	newOK bool,
) {
	if oldOK {
		state.signedSum -= oldRho
		state.absSum -= math.Abs(oldRho)
		state.peerCount--
	}

	if newOK {
		state.signedSum += newRho
		state.absSum += math.Abs(newRho)
		state.peerCount++
	}
}
