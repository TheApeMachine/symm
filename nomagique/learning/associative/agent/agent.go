package agent

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/learning/associative/model"
	"github.com/theapemachine/symm/nomagique/learning/associative/prior"
	"github.com/theapemachine/symm/nomagique/learning/associative/reward"
)

/* Agent owns action learning; Environment owns the resources those actions affect. */
type Choice[Action comparable] struct { Action Action; Prior prior.Reading }

type Activation[Action comparable] struct { Decision *Decision[Action]; Choices []Choice[Action] }

type Agent[Action comparable] struct {
 Activations map[string]*Activation[Action]
	Model       *model.Model[string, Action]
	Environment Environment[Action]
	Explore     bool
	Pending     map[uint64]*Decision[Action]
	Last        *Decision[Action]
	Selected    prior.Reading
	Reward      reward.Outcome
	Outcomes    equation.PriorMoments
	Reading     equation.PriorSummary
	Decisions   uint64
	Positive    uint64
	Negative    uint64
	Err         error
	ledger      reward.Ledger
}

/* New binds an independent model to one environment and an explicit selection mode. */
func New[Action comparable](
	environment Environment[Action], learned *model.Model[string, Action], explore bool,
) (*Agent[Action], error) {
	if environment == nil || learned == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "agent: environment and model required", nil,
		))
	}
	learned.Ordered = true

	return &Agent[Action]{
		Model: learned, Environment: environment, Explore: explore,
		Pending: make(map[uint64]*Decision[Action]), Activations: make(map[string]*Activation[Action]),
	}, nil
}

/*
Activate selects and issues one feasible action under the current and prior
observation context. Undefined evidence remains undefined in Selected.
*/
func (agent *Agent[Action]) Activate(
	label string, at time.Time, context []uint64, authority float64,
) (err error) {
	defer func() { agent.Err = err }()
	actions, state, err := agent.Environment.Feasible(label)

	if err != nil {
		return errnie.Error(err)
	}
	conditioned := make([]uint64, 0, len(state)+len(context))
	conditioned = append(conditioned, state...)
	conditioned = append(conditioned, context...)
	choices := make([]Choice[Action], 0, len(actions))
 action, selected, err := agent.Model.Select(label, conditioned, actions, agent.Explore, func(label string, context []uint64, action Action) prior.Reading {
 reading := agent.Model.Recall(label, context, action)
 choices = append(choices, Choice[Action]{Action: action, Prior: reading})
 return reading
 })

	if err != nil {
		return errnie.Error(err)
	}
	identity, err := agent.Model.Issue(label, conditioned, action, authority)

	if err != nil {
		return errnie.Error(err)
	}
	decision := &Decision[Action]{
		ID: identity, Label: label, At: at, Action: action,
		Context: conditioned, Authority: authority,
	}
	agent.Pending[identity], agent.Last, agent.Selected = decision, decision, selected
	agent.Activations[label] = &Activation[Action]{Decision: decision, Choices: choices}
 agent.Decisions++
	return errnie.Error(agent.Environment.Execute(decision))
}

/*
Measure feeds the latest unresolved action the objective change since the last
mark, adjusted for elapsed time by Ledger's measured prior rate. This is weak
account-level feedback, not a claim that the action caused the whole change.
Identical redelivery never supplies another training sample.
*/
func (agent *Agent[Action]) Measure() (err error) {
	defer func() { agent.Err = err }()
	mark, err := agent.Environment.Objective()

	if err != nil {
		return errnie.Error(err)
	}
	previous := agent.Reward.Through.Version
	outcome, err := agent.ledger.Measure(mark)

	if err != nil {
		return errnie.Error(err)
	}
	agent.Reward = outcome

	if mark.Version == previous || outcome.Transitions == 0 || agent.Last == nil {
		return nil
	}

	if _, pending := agent.Pending[agent.Last.ID]; !pending {
		return nil
	}
	value := outcome.Reward

	if outcome.HasPriorRate {
		value = outcome.Differential
	}

	return agent.Model.Feedback(agent.Last.ID, value)
}

/*
Resolve accepts a completed domain evaluation exactly once. Value uses the same
objective units as provisional feedback; the evaluator owns opportunity maturity
and action attribution. Negative and zero evaluations train this agent too.
*/
func (agent *Agent[Action]) Resolve(identity uint64, value float64) (resolved *Decision[Action], err error) {
	defer func() { agent.Err = err }()
	decision, exists := agent.Pending[identity]

	if !exists {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "agent: evaluation requires an unresolved decision", nil,
		))
	}

	if _, err := agent.Model.Resolve(identity, value); err != nil {
		return nil, errnie.Error(err)
	}

	if err := agent.Outcomes.Observe(value, decision.Authority, 0); err != nil {
		return nil, errnie.Error(err)
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
	defer func() { agent.Err = err }()
	if _, exists := agent.Pending[identity]; !exists {
		return errnie.Error(errnie.Err(
			errnie.Validation, "agent: abort requires an unresolved decision", nil,
		))
	}

	if err := agent.Model.Abort(identity); err != nil {
		return errnie.Error(err)
	}
	delete(agent.Pending, identity)
	return nil
}
