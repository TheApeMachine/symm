package advisor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/vector"
	"github.com/theapemachine/symm/types"
)

/* Solver classifies one declared set of competing Advisor Features. */
type Solver struct {
	*Issuer
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	status        *runtime.Status
	groupSpec     []vector.Group
	classifiers   map[string]*vector.Classifier
	clocks        map[string]float64
	lastIssued    map[string]uint64
	observations  map[string]map[string]float64
	ObserveModule func(string, time.Duration)
}

/*
declaredKeys is how many distinct metrics this advisor's whole evidence
contract names. It gives a missing-key report its denominator: one key short of
a full set and a set that never arrives are different failures.
*/
func (solver *Solver) declaredKeys() int {
	declared := make(map[string]bool)

	for _, group := range solver.groupSpec {
		for _, key := range group.Keys {
			declared[key] = true
		}
	}

	return len(declared)
}

/* NewSolver compiles one class-bearing Feature into each classifier group. */
func NewSolver(ctx context.Context, name string, features []*Feature) *Solver {
	ctx, cancel := context.WithCancel(ctx)
	solver := &Solver{
		ctx:          ctx,
		cancel:       cancel,
		status:       runtime.NewStatus(),
		clocks:       make(map[string]float64),
		lastIssued:   make(map[string]uint64),
		observations: make(map[string]map[string]float64),
		classifiers:  make(map[string]*vector.Classifier),
	}

	if name == "" {
		solver.fail(errnie.Validation, "advisor requires a name", nil)

		return solver
	}

	groups, clock, err := featureGroups(features)

	if err != nil {
		solver.fail(errnie.Validation, "advisor feature contract failed", err)

		return solver
	}

	solver.Issuer = newIssuer(name, features, groups, clock)
	solver.groupSpec = groups
	solver.status.Transition(runtime.READY)

	return solver
}

func (solver *Solver) Error() error            { return solver.err }
func (solver *Solver) Status() *runtime.Status { return solver.status }

/* Step classifies envelopes carrying this Advisor's complete observation set. */
func (solver *Solver) Step(envelope *types.Envelope) *types.Envelope {
	if solver.err != nil || solver.status.Current() == runtime.FATAL ||
		solver.status.Current() == runtime.DONE {
		return nil
	}

	if envelope == nil {
		solver.fail(errnie.BadRequest, "advisor received nil envelope", nil)

		return nil
	}

	symbol := envelopeSymbol(envelope)

	started := time.Now()

	defer func() {
		if solver.ObserveModule != nil {
			solver.ObserveModule("advisor", time.Since(started))
		}
	}()

	input, liftErr := envelope.LiftedObservation()

	if liftErr != nil {
		if len(input) == 0 {
			solver.fail(errnie.UnprocessableContent, "advisor measurement lift failed", liftErr)

			return nil
		}

		errnie.Warn(fmt.Sprintf(
			"[advisor] %s: skipped failed signal measurement: %v",
			solver.name, liftErr,
		))
	}

	clock, clockFound := input[solver.clock]

	if !clockFound {
		if symbol != "" {
			solver.observe(symbol, input)
		}

		if envelope.TypeID == types.EnvelopeTrade &&
			hasClockSourceObservation(input, solver.clock, solver.groups) {
			solver.fail(errnie.PreconditionFailed, "advisor market clock observation is missing", nil)

			return nil
		}

		return envelope
	}

	if symbol == "" {
		solver.fail(errnie.UnprocessableContent, "advisor requires a market symbol", nil)

		return nil
	}

	observation := solver.observe(symbol, input)

	_, err := solver.clockAdvanced(symbol, clock)

	if err != nil {
		solver.fail(errnie.Conflict, "advisor market clock failed", err)

		return nil
	}

	clockOrdinal := uint64(clock)

	if clockOrdinal == 0 {
		return envelope
	}

	lastIssuedOrdinal, hasIssued := solver.lastIssued[symbol]

	if hasIssued && lastIssuedOrdinal >= clockOrdinal {
		return envelope
	}

	solver.status.Transition(runtime.WAITING)

	classifier := solver.classifierFor(symbol)

	if classifier == nil {
		return envelope
	}

	if !classifier.Complete(observation) {
		// An advisor that cannot complete its evidence publishes nothing, and
		// nothing else in the system notices. Recording WHICH declared keys are
		// absent is the difference between "this instrument is quiet" and "this
		// advisor has been structurally mute since the process started".
		envelope.AppendAdvisorSilence(types.AdvisorSilence{
			Advisor:  solver.name,
			Reason:   "incomplete",
			Missing:  classifier.Missing(observation),
			Declared: solver.declaredKeys(),
		})

		return envelope
	}

	solver.status.Transition(runtime.READY)

	if !classifier.Observe(observation) {
		solver.fail(errnie.ExpectationFailed, "advisor classification produced no distribution", nil)

		return nil
	}

	if err := solver.Issue(envelope, classifier.Read(), clockOrdinal); err != nil {
		solver.halt(err)

		return nil
	}

	solver.lastIssued[symbol] = clockOrdinal

	return envelope
}

/* Distribution returns the latest complete class distribution for one symbol. */
func (solver *Solver) Distribution(
	symbol string,
) ([]types.PerspectiveClass, float64, bool, error) {
	if solver.err != nil {
		return nil, 0, false, solver.err
	}

	classifier, found := solver.classifiers[symbol]

	if !found {
		return nil, 0, false, nil
	}

	distribution := classifier.Read()

	if !distribution.Ready {
		return nil, 0, false, errnie.Error(errnie.Err(
			errnie.ExpectationFailed,
			"[advisor] stored classification is incomplete",
			nil,
		))
	}

	classes := make([]types.PerspectiveClass, len(solver.groups))

	for index, group := range solver.groups {
		classes[index] = types.PerspectiveClass{
			State:       types.PerspectiveState(group.Label),
			Probability: distribution.Probabilities[group.Label],
		}
	}

	return classes, distribution.Sharpness, true, nil
}

func featureGroups(features []*Feature) ([]vector.Group, string, error) {
	if len(features) < 2 {
		return nil, "", errnie.Err(
			errnie.Validation,
			"Advisor requires at least two competing Features",
			nil,
		)
	}

	labels := make(map[string]bool, len(features))
	groups := make([]vector.Group, len(features))
	clock := ""
	predictive := false
	within := uint64(0)

	for index, feature := range features {
		if feature == nil || feature.Class == nil || feature.Class.Label == "" {
			return nil, "", errnie.Err(
				errnie.Validation,
				"Advisor Feature requires exactly one named Class",
				nil,
			)
		}

		if labels[feature.Class.Label] {
			return nil, "", errnie.Err(
				errnie.Validation,
				"Advisor class label must be unique: "+feature.Class.Label,
				nil,
			)
		}

		if err := validateFeatureKeys(feature); err != nil {
			return nil, "", errnie.Err(
				errnie.Validation,
				"[advisor] failed to validate feature keys for: "+feature.Class.Label,
				err,
			)
		}

		if err := feature.validatePredictions(); err != nil {
			return nil, "", err
		}

		if index == 0 {
			predictive = len(feature.Class.Predictions) > 0
			within = feature.Class.Within
		}

		if index > 0 && ((len(feature.Class.Predictions) > 0) != predictive ||
			feature.Class.Within != within) {
			return nil, "", errnie.Err(
				errnie.Validation,
				"Advisor Classes require one shared prediction horizon and complete declarations",
				nil,
			)
		}

		if clock == "" {
			clock = feature.Clock
		}

		if feature.Clock == "" || feature.Clock != clock {
			return nil, "", errnie.Err(
				errnie.Validation,
				"Advisor Features require one shared market clock",
				nil,
			)
		}

		labels[feature.Class.Label] = true
		groups[index] = vector.NewGroup(feature.Class.Label, feature.Keys...)
	}

	return groups, clock, nil
}

func validateFeatureKeys(feature *Feature) error {
	if len(feature.Keys) == 0 {
		return errnie.Err(
			errnie.Validation,
			"Advisor Feature requires metric keys",
			nil,
		)
	}

	keys := make(map[string]bool, len(feature.Keys))

	for _, key := range feature.Keys {
		source, metric, qualified := strings.Cut(key, "/")

		if !qualified || source == "" || metric == "" || strings.Contains(metric, "/") {
			return errnie.Err(
				errnie.Validation,
				"Advisor metric key must be source-qualified: "+key,
				nil,
			)
		}

		if keys[key] {
			return errnie.Err(
				errnie.Validation,
				"Advisor Feature repeats metric key: "+key,
				nil,
			)
		}

		keys[key] = true
	}

	return nil
}

/*
observe merges one lifted observation into the symbol's retained evidence and
returns it. Metrics arrive across many envelopes, so the observation
accumulates until every declared metric is present.
*/
func (solver *Solver) observe(
	symbol string,
	input map[string]float64,
) map[string]float64 {
	observation, found := solver.observations[symbol]

	if !found {
		observation = make(map[string]float64)
		solver.observations[symbol] = observation
	}

	for key, value := range input {
		observation[key] = value
	}

	return observation
}

/*
classifierFor returns the symbol's classifier, compiling one on first use so
each symbol's standardizers accumulate their own causal history.
*/
func (solver *Solver) classifierFor(symbol string) *vector.Classifier {
	classifier, found := solver.classifiers[symbol]

	if !found {
		var err error
		classifier, err = vector.NewClassifier(solver.groupSpec...)

		if err != nil {
			solver.fail(errnie.Internal, "advisor failed to compile classifier", err)

			return nil
		}

		solver.classifiers[symbol] = classifier
	}

	return classifier
}

/*
hasClockSourceObservation reports whether the observation carries a metric
from the same source as the declared market clock. A clock source that is
speaking but has not stated its clock is a contract failure, not a wait.
*/
func hasClockSourceObservation(
	input map[string]float64,
	clock string,
	groups []vector.Group,
) bool {
	clockSource, _, hasSlash := strings.Cut(clock, "/")

	if !hasSlash {
		return false
	}

	for _, group := range groups {
		for _, key := range group.Keys {
			source, _, _ := strings.Cut(key, "/")

			if source != clockSource {
				continue
			}

			if _, present := input[key]; present {
				return true
			}
		}
	}

	return false
}

func (solver *Solver) clockAdvanced(symbol string, clock float64) (bool, error) {
	if clock < 0 || float64(uint64(clock)) != clock {
		return false, errnie.Err(
			errnie.Validation,
			"Advisor market clock must be a non-negative ordinal",
			nil,
		)
	}

	previous, found := solver.clocks[symbol]

	if found && clock < previous {
		return false, errnie.Err(
			errnie.Validation,
			"Advisor market clock moved backwards",
			nil,
		)
	}

	solver.clocks[symbol] = clock

	if !found {
		return clock > 0, nil
	}

	return clock > previous, nil
}

func (solver *Solver) fail(kind errnie.Kind, message string, err error) {
	solver.halt(errnie.Error(errnie.Err(kind, "[advisor] "+message, err)))
}

func (solver *Solver) halt(err error) {
	solver.err = err
	solver.status.Transition(runtime.FATAL)
	solver.cancel()
}

/*
underlyingSymbol resolves a futures product to the spot market it derives from,
falling back to the product identity when the venue's naming does not cover it.
The fallback keeps such an instrument observable under its own name rather than
discarding it.
*/
func underlyingSymbol(product string) string {
	if spot, ok := kraken.SpotSymbol(product); ok {
		return spot
	}

	return product
}

func envelopeSymbol(envelope *types.Envelope) string {
	switch envelope.TypeID {
	case types.EnvelopeTicker:
		return envelope.TickerData.Symbol
	case types.EnvelopeTrade:
		return envelope.TradeData.Symbol
	case types.EnvelopeLevel3:
		return envelope.Level3Data.Symbol
	// A futures envelope carries the derivative's own product identity, and an
	// advisor accumulates its evidence per symbol. Filed under "PF_SOLUSD" the
	// derivative facts never meet the spot facts filed under "SOL/USD", so an
	// advisor mixing the two — Basis declares nothing but derivatives metrics
	// and the spot volume-bar clock — can never assemble a complete set for any
	// instrument. Resolving to the underlying is what lets the two meet.
	case types.EnvelopeFuturesTicker:
		return underlyingSymbol(envelope.FuturesTickerData.Symbol)
	case types.EnvelopeFuturesTrade:
		return underlyingSymbol(envelope.FuturesTradeData.Symbol)
	default:
		return ""
	}
}

/* Close releases the Solver after its final classification. */
func (solver *Solver) Close() error {
	solver.cancel()

	if solver.status.Current() != runtime.FATAL {
		solver.status.Transition(runtime.DONE)
	}

	return solver.err
}
