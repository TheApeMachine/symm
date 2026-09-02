package advisor

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/vector"
	"github.com/theapemachine/symm/types"
)

/* Solver classifies one declared set of competing Advisor Features. */
type Solver struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	status        *runtime.Status
	number        *nomagique.Number[string]
	groups        []vector.Group
	clock         nmtypes.Symbol
	clocks        map[string]float64
	ObserveModule func(string, time.Duration)
}

/* NewSolver compiles one class-bearing Feature into each classifier group. */
func NewSolver(ctx context.Context, features []*Feature) *Solver {
	ctx, cancel := context.WithCancel(ctx)
	solver := &Solver{
		ctx:    ctx,
		cancel: cancel,
		status: runtime.NewStatus(),
		clocks: make(map[string]float64),
	}

	groups, clock, err := featureGroups(features)

	if err != nil {
		solver.fail("Advisor feature contract failed", err)

		return solver
	}

	solver.groups = groups
	solver.clock = clock
	solver.number = nomagique.NewNumber[string](
		vector.AdaptiveClassifier(groups...),
	)
	solver.status.Transition(runtime.READY)

	return solver
}

func (solver *Solver) Name() string            { return "advisor" }
func (solver *Solver) Error() error            { return solver.err }
func (solver *Solver) Status() *runtime.Status { return solver.status }

/* Step classifies envelopes carrying this Advisor's complete observation set. */
func (solver *Solver) Step(envelope *types.Envelope) *types.Envelope {
	if solver.err != nil || solver.status.Current() == runtime.FATAL ||
		solver.status.Current() == runtime.DONE {
		return nil
	}

	if envelope == nil {
		solver.fail("Advisor received nil envelope", nil)

		return nil
	}

	symbol := envelopeSymbol(envelope)

	started := time.Now()

	defer func() {
		if solver.ObserveModule != nil {
			solver.ObserveModule("advisor", time.Since(started))
		}
	}()

	input := data.Lift(envelope.SignalMeasurements())

	if input.Err != nil {
		solver.fail("Advisor measurement lift failed", input.Err)

		return nil
	}

	clock, clockFound := input.Get(solver.clock)

	if !clockFound && !hasFeatureObservation(input, solver.groups) {
		return envelope
	}

	if !clockFound {
		solver.fail("Advisor market clock observation is missing", nil)

		return nil
	}

	if symbol == "" {
		solver.fail("Advisor requires a market symbol", nil)

		return nil
	}

	advanced, err := solver.clockAdvanced(symbol, clock)

	if err != nil {
		solver.fail("Advisor market clock failed", err)

		return nil
	}

	if !advanced {
		return envelope
	}

	solver.status.Transition(runtime.WAITING)

	if !vector.GroupsComplete(&input, solver.groups) {
		return envelope
	}

	solver.status.Transition(runtime.READY)

	output := solver.number.Step(symbol, input)

	if output.Err != nil {
		solver.fail("Advisor classification failed", output.Err)

		return nil
	}

	ready, found := output.Get(nmtypes.SampleReady)

	if !found || ready != 1 {
		solver.fail("Advisor classification produced no distribution", nil)

		return nil
	}

	return envelope
}

/* Distribution returns the latest complete class distribution for one symbol. */
func (solver *Solver) Distribution(
	symbol string,
) ([]types.PerspectiveClass, float64, bool, error) {
	if solver.err != nil {
		return nil, 0, false, solver.err
	}

	frame, found := solver.number.Project(symbol)

	if !found {
		return nil, 0, false, nil
	}

	distribution := vector.Unpack(frame, solver.groups)

	if !distribution.Ready {
		return nil, 0, false, errnie.Err(
			errnie.Validation,
			"Advisor stored classification is incomplete",
			nil,
		)
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

func featureGroups(features []*Feature) ([]vector.Group, nmtypes.Symbol, error) {
	if len(features) < 2 {
		return nil, 0, errnie.Err(
			errnie.Validation,
			"Advisor requires at least two competing Features",
			nil,
		)
	}

	labels := make(map[string]bool, len(features))
	groups := make([]vector.Group, len(features))
	clock := ""

	for index, feature := range features {
		if feature == nil || feature.Class == nil || feature.Class.Label == "" {
			return nil, 0, errnie.Err(
				errnie.Validation,
				"Advisor Feature requires exactly one named Class",
				nil,
			)
		}

		if labels[feature.Class.Label] {
			return nil, 0, errnie.Err(
				errnie.Validation,
				"Advisor class label must be unique: "+feature.Class.Label,
				nil,
			)
		}

		if err := validateFeatureKeys(feature); err != nil {
			return nil, 0, err
		}

		if clock == "" {
			clock = feature.Clock
		}

		if feature.Clock == "" || feature.Clock != clock {
			return nil, 0, errnie.Err(
				errnie.Validation,
				"Advisor Features require one shared market clock",
				nil,
			)
		}

		labels[feature.Class.Label] = true
		groups[index] = vector.NewGroup(feature.Class.Label, feature.Keys...)
	}

	return groups, nmtypes.MustIntern(clock), nil
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

func hasFeatureObservation(input nmtypes.Frame, groups []vector.Group) bool {
	for _, group := range groups {
		if slices.ContainsFunc(group.Symbols, input.Has) {
			return true
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

func (solver *Solver) fail(message string, err error) {
	solver.err = errnie.Error(errnie.Err(errnie.Validation, message, err))
	solver.status.Transition(runtime.FATAL)
	solver.cancel()
}

func envelopeSymbol(envelope *types.Envelope) string {
	switch envelope.TypeID {
	case types.EnvelopeTicker:
		return envelope.TickerData.Symbol
	case types.EnvelopeTrade:
		return envelope.TradeData.Symbol
	case types.EnvelopeLevel3:
		return envelope.Level3Data.Symbol
	case types.EnvelopeFuturesTicker:
		return envelope.FuturesTickerData.Symbol
	case types.EnvelopeFuturesTrade:
		return envelope.FuturesTradeData.Symbol
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
