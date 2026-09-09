package agent

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/learning/associative/reward"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* Agent owns precursor inference and its unresolved actions. */
type Agent[Action comparable] struct {
	*runtime.System
	Activations map[string]*Decision[Action]
	Model       *cognition.Engine
	Environment Environment[Action]
	Explore     bool
	Pending     map[uint64]*Decision[Action]
	Last        *Decision[Action]
	Reward      reward.Outcome
	Outcomes    equation.PriorMoments
	Reading     equation.PriorSummary
	Decisions   uint64
	Positive    uint64
	Negative    uint64
	ledger      reward.Ledger
	histories   map[string][]transition
	sequence    uint64
}

/* New binds a model to a rehearsal explorer or the accumulated forward-test agent. */
func New[Action comparable](
	ctx context.Context, environment Environment[Action], learned *cognition.Engine, explore bool,
) (*Agent[Action], error) {
	if environment == nil || learned == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "agent: environment and model required", nil,
		))
	}

	return &Agent[Action]{
		System: runtime.NewSystem(ctx, "agent"),
		Model:  learned, Environment: environment, Explore: explore,
		Pending: make(map[uint64]*Decision[Action]), Activations: make(map[string]*Decision[Action]),
	}, nil
}

// Step consumes the grid's completed regions. Both live and replay paths use
// this operation; market readiness remains the enclosing workload's dependency.
func (agent *Agent[Action]) Step(impulse grid.Impulse) grid.Impulse {
	if agent.Error() != nil {
		return impulse
	}

	if !impulse.Ready {
		agent.Transition(runtime.WAITING)
		return impulse
	}
	conditions := make([]uint64, 0, len(impulse.Regions))
	strength, authority := 0.0, 0.0
	for _, region := range impulse.Regions {
		conditions = append(conditions, region.Condition)
		strength += region.Strength
		authority += region.Strength * region.Authority
	}

	if strength == 0 {
		agent.Transition(runtime.WAITING)
		return impulse
	}

	if err := agent.Measure(); err != nil {
		return impulse
	}
	sequence := agent.Context(impulse.Label, impulse.At, impulse.From, conditions)
	agent.Fail(agent.Activate(impulse.Label, impulse.At, sequence, authority/strength))
	return impulse
}

// Activate evaluates the original precursor sequence and issues one feasible
// action. Explorers sample when evidence is uncertain; live inference abstains
// when there is no supported feasible winner. Abstention is not a wait sample.
func (agent *Agent[Action]) Activate(
	label string, at time.Time, context []uint64, authority float64,
) (err error) {
	defer func() { agent.Fail(err) }()
	actions, state, err := agent.Environment.Feasible(label)

	if err != nil {
		return errnie.Error(err)
	}

	if len(actions) < 2 {
		agent.Transition(runtime.WAITING)
		return nil
	}
	conditioned := append(append(make([]uint64, 0, len(state)+len(context)), state...), context...)
	evaluation := agent.Model.Evaluate(ContextKey(label, conditioned))

	if evaluation.IsBreak {
		delete(agent.histories, label)
	}
	selected := -1
	for index, action := range actions {
		if fmt.Sprint(action) == evaluation.WinnerClass {
			selected = index
		}
	}

	if agent.Explore && (selected < 0 || evaluation.RunnerUp == "" || rand.Float64() < evaluation.Ambiguity) {
		selected = rand.IntN(len(actions))
	}

	if selected < 0 || (!agent.Explore && evaluation.RunnerUp != "" && evaluation.Contrast == 0) {
		agent.Transition(runtime.WAITING)
		return nil
	}
	agent.sequence++
	decision := &Decision[Action]{
		ID: agent.sequence, Label: label, At: at, Action: actions[selected],
		Context: conditioned, Authority: authority, Evaluation: evaluation,
		Alternatives: slices.Clone(actions),
	}
	agent.Pending[decision.ID], agent.Last = decision, decision
	agent.Activations[label] = decision
	agent.Decisions++
	agent.Transition(runtime.READY)
	return errnie.Error(agent.Environment.Execute(decision))
}

/*
Measure records account economics for evaluation and telemetry. A wallet mark
is not a causal grade and never updates cognition.
*/
func (agent *Agent[Action]) Measure() (err error) {
	defer func() { agent.Fail(err) }()
	mark, err := agent.Environment.Objective()

	if err != nil {
		return errnie.Error(err)
	}

	if mark == nil {
		return nil
	}
	outcome, err := agent.ledger.Measure(*mark)

	if err != nil {
		return errnie.Error(err)
	}
	agent.Reward = outcome
	return nil
}

/*
Resolve accepts a completed evaluation exactly once. Rehearsal explorers
train from signed failure grades; the accumulated forward agent records its
independent evaluation without teaching its different tape-return score back
into the rehearsal policy. Wallet marks never update either model.
*/
func (agent *Agent[Action]) Resolve(identity uint64, value float64) (resolved *Decision[Action], err error) {
	defer func() { agent.Fail(err) }()
	decision, exists := agent.Pending[identity]

	if !exists {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "agent: evaluation requires an unresolved decision", nil,
		))
	}

	if err := agent.Outcomes.Observe(value, decision.Authority, 0); err != nil {
		return nil, errnie.Error(err)
	}

	if agent.Explore && decision.Authority > 0 {
		agent.Model.Observe(ContextKey(decision.Label, decision.Context),
			[]byte(fmt.Sprint(decision.Action)), value*decision.Authority)
	}
	decision.Outcome = &value
	agent.Reading = agent.Outcomes.Summary(0)
	delete(agent.Pending, identity)

	if value > 0 {
		agent.Positive++
	}

	if value < 0 {
		agent.Negative++
	}

	return decision, nil
}

/* Abort is explicit confirmation that an issued operation did not take effect. */
func (agent *Agent[Action]) Abort(identity uint64) (err error) {
	defer func() { agent.Fail(err) }()
	_, exists := agent.Pending[identity]

	if !exists {
		return errnie.Error(errnie.Err(
			errnie.Validation, "agent: abort requires an unresolved decision", nil,
		))
	}

	delete(agent.Pending, identity)
	return nil
}

// ContextKey encodes a symbol and complete condition tokens without allowing
// prefix matching inside a token. NUL terminates each identity.
func ContextKey(label string, context []uint64) []byte {
	result := append([]byte(label), 0)
	for _, token := range context {
		result = strconv.AppendUint(result, token, 16)
		result = append(result, 0)
	}
	return result
}
