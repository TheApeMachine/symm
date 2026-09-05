package learning

import (
	"github.com/theapemachine/errnie"
)

/*
Model keeps priors for keyed, ordered contexts and comparable actions. Context
tokens are stable numerical identities supplied by the caller, in order. They
may identify regions or previous-action context; the model interprets neither.
Action includes its parameters in its comparable identity.

The context trie interns each distinct prefix once. Repeated decisions reuse
its prior records, and pending decisions retain a node reference rather than
mutable grid coordinates or a copied input vector. One owner serializes access.
*/
type Model[Key comparable, Action comparable] struct {
	contexts map[Key]*modelContext[Action]
	pending  map[uint64]pendingAction
	sequence uint64
}

/* modelContext owns the actions and continuations of one context prefix. */
type modelContext[Action comparable] struct {
	children map[uint64]*modelContext[Action]
	priors   map[Action]*Prior
}

/* pendingAction holds only the evidence fixed when an action was issued. */
type pendingAction struct {
	prior     *Prior
	authority float64
}

/* NewModel constructs a keyed prior model without tuning parameters. */
func NewModel[Key comparable, Action comparable]() *Model[Key, Action] {
	return &Model[Key, Action]{
		contexts: make(map[Key]*modelContext[Action]),
		pending:  make(map[uint64]pendingAction),
	}
}

/*
Issue binds an action to its ordered context and numeric observation authority.
The discovery producer supplies authority in [0, 1]; the model assigns no
semantic meaning to its inputs. Unresolved actions do not train zero outcomes.
Input reuse cannot rewrite the context or authority fixed at issue time.
*/
func (model *Model[Key, Action]) Issue(
	key Key, context []uint64, action Action, authority float64,
) (uint64, error) {
	if authority < 0 || authority > 1 {
		return 0, errnie.Err(errnie.Validation, "model: authority must be in [0, 1]", nil)
	}

	node := model.contexts[key]

	if node == nil {
		node = &modelContext[Action]{}
		model.contexts[key] = node
	}

	for _, token := range context {
		if node.children == nil {
			node.children = make(map[uint64]*modelContext[Action])
		}

		next := node.children[token]

		if next == nil {
			next = &modelContext[Action]{}
			node.children[token] = next
		}

		node = next
	}

	if node.priors == nil {
		node.priors = make(map[Action]*Prior)
	}

	prior := node.priors[action]

	if prior == nil {
		prior = &Prior{}
		node.priors[action] = prior
	}

	model.sequence++
	prior.pending++
	model.pending[model.sequence] = pendingAction{
		prior: prior, authority: authority,
	}

	return model.sequence, nil
}

/*
Resolve incorporates an issued action's outcome exactly once and releases its
pending record. The caller supplies the numerical target assigned to this
decision, for example its subsequent reward or return-to-go. Shared sequence
returns are correlated targets, not independent evidence of each action's
causal effect. This method estimates assigned targets, not causality.
*/
func (model *Model[Key, Action]) Resolve(identity uint64, outcome float64) (PriorReading, error) {
	pending, exists := model.pending[identity]

	if !exists {
		return PriorReading{}, errnie.Err(
			errnie.Validation, "model: action was not issued or is already resolved", nil,
		)
	}

	if err := pending.prior.Observe(outcome, pending.authority); err != nil {
		return PriorReading{}, err
	}

	delete(model.pending, identity)
	pending.prior.pending--

	return pending.prior.Reading(), nil
}

/*
Recall returns the aggregate of all completed matches for this exact key,
ordered context and action including parameters. Lookup does not allocate or
create evidence. Different orders, prefixes, keys and actions remain distinct;
approximate matching is not silently substituted for an absent exact match.
*/
func (model *Model[Key, Action]) Recall(key Key, context []uint64, action Action) PriorReading {
	node := model.contexts[key]

	for _, token := range context {
		if node == nil {
			return PriorReading{}
		}

		node = node.children[token]
	}

	if node == nil || node.priors[action] == nil {
		return PriorReading{}
	}

	return node.priors[action].Reading()
}
