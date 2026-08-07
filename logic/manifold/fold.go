package manifold

/*
foldReportInterval is how many advances pass between fold reports. The measurement is
cumulative, so the interval only controls how often it is read out.
*/
const foldReportInterval = 16

/*
excitationState records why a side did or did not receive forcing, so a manifold that
never drives can be told apart from one whose market is simply quiet.
*/
type excitationState int

const (
	// excitationForced is a converged fit reporting arrivals above baseline.
	excitationForced excitationState = iota
	// excitationUnfit is a hawkes fit that has not converged, publishing a zero placeholder.
	excitationUnfit
	// excitationBelowBaseline is a converged fit whose arrivals sit under their own
	// immigrant baseline, which is an ordinary quiet market rather than a fault.
	excitationBelowBaseline
	// excitationMissing is an absent metric key, which should not happen.
	excitationMissing
)

/*
foldMeter counts how much of the injected population the inelastic merge actually
absorbs.

Why:

	"Collision is compression" is the mechanism that is supposed to turn repeated
	observations into single heavy particles, and mass is only evidence if that merge
	fires. Nothing in the pipeline reports whether it does, so a manifold that never
	folds anything is indistinguishable from one that folds correctly.

	The three counters have to be separated because they fail differently. A fold rate
	near zero means the content identities never collide, so the population is a stream
	of strangers and mass never accumulates. A fold rate near one means identities
	collide too easily and distinct observations are being summed into each other.
	Eviction is neither: it is the residency cap discarding the oldest particles by
	policy, which suppresses the resident count for reasons that have nothing to do with
	compression, and would otherwise be misread as folding.
*/
type foldMeter struct {
	injected uint64
	merged   uint64
	evicted  uint64
	advances uint64
	cap      int
	drive    [4]uint64
}

/*
excite tallies one side's forcing outcome.
*/
func (meter *foldMeter) excite(state excitationState) {
	if int(state) < len(meter.drive) {
		meter.drive[state]++
	}
}

/*
inject records particles handed to the domain.
*/
func (meter *foldMeter) inject(count int) {
	if count > 0 {
		meter.injected += uint64(count)
	}
}

/*
fold records the population lost across one advance. Inelastic merge is the only stage
inside Advance that removes particles, so the shrinkage is the merge yield.
*/
func (meter *foldMeter) fold(before, after int) {
	meter.advances++

	if before > after {
		meter.merged += uint64(before - after)
	}
}

/*
drop records the population discarded by the residency cap, keeping policy eviction out
of the merge yield.
*/
func (meter *foldMeter) drop(before, after int) {
	if before > after {
		meter.evicted += uint64(before - after)
	}
}

/*
due reports whether a readout is owed.
*/
func (meter *foldMeter) due() bool {
	return meter.advances > 0 && meter.advances%foldReportInterval == 0
}
