package advisor

import (
	"context"
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
	*Issuer
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	status        *runtime.Status
	number        *nomagique.Number[string]
	clocks        map[string]float64
	frames        map[string]nmtypes.Frame
	ObserveModule func(string, time.Duration)
}

/* NewSolver compiles one class-bearing Feature into each classifier group. */
func NewSolver(ctx context.Context, name string, features []*Feature) *Solver {
	ctx, cancel := context.WithCancel(ctx)
	solver := &Solver{
		ctx:    ctx,
		cancel: cancel,
		status: runtime.NewStatus(),
		clocks: make(map[string]float64),
		frames: make(map[string]nmtypes.Frame),
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
	solver.number = nomagique.NewNumber[string](
		vector.AdaptiveClassifier(groups...),
	)
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

	input := data.Lift(envelope.SignalMeasurements())

	if input.Err != nil {
		solver.fail(errnie.UnprocessableContent, "advisor measurement lift failed", input.Err)

		return nil
	}

	clock, clockFound := input.Get(solver.clock)

	if !clockFound {
		if symbol != "" {
			frame := solver.frames[symbol]
			frame.Merge(input)
			solver.frames[symbol] = frame
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

	frame := solver.frames[symbol]
	frame.Merge(input)
	solver.frames[symbol] = frame

	advanced, err := solver.clockAdvanced(symbol, clock)

	if err != nil {
		solver.fail(errnie.Conflict, "advisor market clock failed", err)

		return nil
	}

	if !advanced {
		return envelope
	}

	solver.status.Transition(runtime.WAITING)

	if !vector.GroupsComplete(&frame, solver.groups) {
		return envelope
	}

	solver.status.Transition(runtime.READY)

	output := solver.number.Step(symbol, frame)

	if output.Err != nil {
		solver.fail(errnie.Internal, "advisor classification failed", output.Err)

		return nil
	}

	ready, found := output.Get(nmtypes.SampleReady)

	if !found || ready != 1 {
		solver.fail(errnie.ExpectationFailed, "advisor classification produced no distribution", nil)

		return nil
	}

	if err := solver.Issue(envelope, output, uint64(clock)); err != nil {
		solver.halt(err)

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
	predictive := false
	within := uint64(0)

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
			return nil, 0, errnie.Err(
				errnie.Validation,
				"[advisor] failed to validate feature keys for: "+feature.Class.Label,
				err,
			)
		}

		if err := feature.validatePredictions(); err != nil {
			return nil, 0, err
		}

		if index == 0 {
			predictive = len(feature.Class.Predictions) > 0
			within = feature.Class.Within
		}

		if index > 0 && ((len(feature.Class.Predictions) > 0) != predictive ||
			feature.Class.Within != within) {
			return nil, 0, errnie.Err(
				errnie.Validation,
				"Advisor Classes require one shared prediction horizon and complete declarations",
				nil,
			)
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

func hasClockSourceObservation(
	input nmtypes.Frame,
	clock nmtypes.Symbol,
	groups []vector.Group,
) bool {
	clockName, found := nmtypes.SymbolName(clock)

	if !found {
		return false
	}

	clockSource, _, hasSlash := strings.Cut(clockName, "/")

	if !hasSlash {
		return false
	}

	for _, group := range groups {
		for _, symbol := range group.Symbols {
			name, nameFound := nmtypes.SymbolName(symbol)

			if !nameFound {
				continue
			}

			source, _, _ := strings.Cut(name, "/")

			if source == clockSource && input.Has(symbol) {
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
