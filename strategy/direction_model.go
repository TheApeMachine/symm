package strategy

import (
	"math"
	"time"

	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

/* weightedAssociation retains the sufficient statistics for one feature/outcome relationship. */
type weightedAssociation struct {
	weightSum   float64
	weightSqSum float64
	meanValue   float64
	meanOutcome float64
	valueM2     float64
	outcomeM2   float64
	coMoment    float64
}

func (association *weightedAssociation) observe(value, outcome, weight float64) {
	if weight <= 0 {
		return
	}

	nextWeight := association.weightSum + weight
	valueDelta := value - association.meanValue
	outcomeDelta := outcome - association.meanOutcome
	association.meanValue += weight / nextWeight * valueDelta
	association.meanOutcome += weight / nextWeight * outcomeDelta
	association.valueM2 += weight * valueDelta * (value - association.meanValue)
	association.outcomeM2 += weight * outcomeDelta * (outcome - association.meanOutcome)
	association.coMoment += weight * valueDelta * (outcome - association.meanOutcome)
	association.weightSum = nextWeight
	association.weightSqSum += weight * weight
}

func (association weightedAssociation) evidence(value float64) (float64, float64, bool) {
	if association.weightSum <= 0 || association.weightSqSum <= 0 ||
		association.valueM2 <= 0 || association.outcomeM2 <= 0 {
		return 0, 0, false
	}

	effectiveSupport := association.weightSum * association.weightSum / association.weightSqSum

	if effectiveSupport <= 1 {
		return 0, 0, false
	}

	correlation := association.coMoment / math.Sqrt(association.valueM2*association.outcomeM2)
	variance := association.valueM2 / association.weightSum
	standardized := (value - association.meanValue) / math.Sqrt(variance)
	maturity := 1 - 1/effectiveSupport

	return math.Tanh(correlation * standardized), maturity, true
}

/*
returnHead calibrates one bounded contextual score against an executable-bid
log return. The RLS posterior is itself the forecast distribution; no binary
classifier or admission score is layered on top of it.
*/
type returnHead struct {
	learner  *learning.RLS
	outcomes uint64
}

func newReturnHead(config directionalConfig) (*returnHead, error) {
	learner, err := learning.NewRLS(learning.RLSConfig{
		Dimension:        1,
		InitialVariance:  config.initialVariance,
		ForgettingFactor: config.forgettingFactor,
	})

	if err != nil {
		return nil, err
	}

	return &returnHead{learner: learner}, nil
}

func (head *returnHead) observe(score, logReturn float64) error {
	if _, err := head.learner.Observe(learning.RLSSample{
		Features: []float64{score},
		Target:   logReturn,
	}); err != nil {
		return err
	}

	head.outcomes++

	return nil
}

func (head *returnHead) predict(score float64) (learning.RLSOutput, error) {
	return head.learner.Predict([]float64{score})
}

func (intervals *intervalMean) observe(at time.Time) {
	if intervals.last.IsZero() {
		intervals.last = at
		return
	}

	delta := at.Sub(intervals.last)
	intervals.last = at

	if delta <= 0 {
		return
	}

	intervals.count++
	intervals.mean += time.Duration((int64(delta) - int64(intervals.mean)) / int64(intervals.count))
}

func (intervals intervalMean) steps(horizon time.Duration) int {
	if intervals.mean <= 0 || horizon <= 0 {
		return 0
	}

	return int(math.Ceil(float64(horizon) / float64(intervals.mean)))
}

func (state *directionalState) observeHorizon(seconds float64) {
	if seconds <= 0 {
		return
	}

	state.horizon = time.Duration(seconds * float64(time.Second))
}

func (state *directionalState) observe(
	key featureKey,
	value, quality float64,
	observedAt time.Time,
	use featureUse,
) {
	feature := state.features[key]

	if feature == nil {
		groupName := key.family + "\x00" + key.source
		group, found := state.groupsByName[groupName]

		if !found {
			group = len(state.groups)
			state.groupsByName[groupName] = group
			state.groups = append(state.groups, evidenceGroup{})
		}

		feature = &predictorFeature{
			group:        group,
			associations: make(map[forecastContext]*weightedAssociation),
			use:          use,
		}
		state.features[key] = feature
	}

	feature.value = value
	feature.quality = quality
	feature.observedAt = observedAt
	feature.observed = true
}

func (state *directionalState) aggregate(
	context forecastContext,
	at time.Time,
	horizon time.Duration,
) (float64, int) {
	for index := range state.groups {
		state.groups[index] = evidenceGroup{}
	}

	for _, feature := range state.features {
		if feature.use != featureContext || !feature.current(at, horizon) {
			continue
		}

		association := feature.associations[context]

		if association == nil {
			continue
		}

		evidence, maturity, ready := association.evidence(feature.value)

		if !ready {
			continue
		}

		weight := maturity * feature.quality
		group := &state.groups[feature.group]
		group.numerator += weight * evidence
		group.denominator += weight
		group.features++
	}

	numerator := 0.0
	denominator := 0.0
	features := 0

	for _, group := range state.groups {
		if group.denominator <= 0 {
			continue
		}

		groupEvidence := group.numerator / group.denominator
		groupMaturity := group.denominator / float64(group.features)
		numerator += groupMaturity * groupEvidence
		denominator += groupMaturity
		features += group.features
	}

	if denominator == 0 {
		return 0, 0
	}

	return numerator / denominator, features
}

func (feature *predictorFeature) current(at time.Time, horizon time.Duration) bool {
	if !feature.observed || feature.quality <= 0 || feature.observedAt.IsZero() {
		return false
	}

	age := at.Sub(feature.observedAt)

	return age >= 0 && age <= horizon
}

func (state *directionalState) resolve(at time.Time, executableBid float64) error {
	if !state.pending.present || at.Sub(state.pending.issuedAt) < state.pending.horizon {
		return nil
	}

	logReturn := math.Log(executableBid / state.pending.reference)

	for _, feature := range state.features {
		if !feature.pending {
			continue
		}

		association := feature.associations[state.pending.context]

		if association == nil {
			association = &weightedAssociation{}
			feature.associations[state.pending.context] = association
		}

		association.observe(feature.pendingValue, logReturn, feature.pendingWeight)
		feature.pending = false
	}

	if err := state.returns.observe(state.pending.score, logReturn); err != nil {
		return err
	}

	state.pending.present = false

	return nil
}

func (state *directionalState) issue(
	at time.Time,
	reference float64,
	context forecastContext,
	score float64,
) {
	if state.pending.present || reference <= 0 {
		return
	}

	state.pending = pendingReturn{
		issuedAt:  at,
		reference: reference,
		horizon:   state.horizon,
		score:     score,
		context:   context,
		present:   true,
	}

	for _, feature := range state.features {
		if feature.use != featureContext || !feature.current(at, state.horizon) {
			continue
		}

		feature.pendingValue = feature.value
		feature.pendingWeight = feature.quality
		feature.pending = true
	}
}

func (state *directionalState) context() (forecastContext, bool) {
	context := forecastContext{
		archetype: state.opportunity.Archetype,
		phase:     state.opportunity.Phase,
	}

	switch state.opportunity.Phase {
	case types.PhaseForming, types.PhaseArmed, types.PhaseIgnition:
		return context, state.opportunity.Direction == types.DirectionLong
	default:
		return context, false
	}
}

func (state *directionalState) currentFeatureCount(use featureUse, at time.Time) int {
	if state.horizon <= 0 {
		return 0
	}

	count := 0

	for _, feature := range state.features {
		if feature.use == use && feature.current(at, state.horizon) {
			count++
		}
	}

	return count
}
