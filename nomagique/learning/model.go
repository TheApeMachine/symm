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
	epoch    uint64
	memory   float64
}

/* modelContext owns the actions and continuations of one context prefix. */
type modelContext[Action comparable] struct {
	children map[uint64]*modelContext[Action]
	priors   map[Action]*Prior
}

/*
pendingAction holds only the evidence fixed when an action was issued. It
retains one prior per context depth, because a decision is evidence about
every prefix of the context it was taken under, not only the longest one.
*/
type pendingAction struct {
	priors    []*Prior
	authority float64
}

/* NewModel constructs a keyed prior model with an optional exponential memory window. */
func NewModel[Key comparable, Action comparable](memory ...float64) *Model[Key, Action] {
	model := &Model[Key, Action]{
		contexts: make(map[Key]*modelContext[Action]),
		pending:  make(map[uint64]pendingAction),
	}

	if len(memory) > 0 && memory[0] > 1 {
		model.memory = memory[0]
	}

	return model
}

/*
Issue binds an action to its ordered context and numeric observation authority.
The discovery producer supplies authority in [0, 1]; the model assigns no
semantic meaning to its inputs. Unresolved actions do not train zero outcomes.
Input reuse cannot rewrite the context or authority fixed at issue time.

The outcome trains this action at every prefix of the context, from the empty
one to the whole sequence. A long context is precise but rare: on its own, a
context of several jittering identities almost never repeats, so no prior ever
reaches a second observation and the model can never leave exploration. Every
prefix carries the same evidence at a coarser resolution, and Recall reads the
deepest usable reading with competitive retained authority, so precision
yields when broader evidence is stronger or fresher.
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

	priors := make([]*Prior, 0, len(context)+1)
	priors = append(priors, node.prior(action, model.memory))

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
		priors = append(priors, node.prior(action, model.memory))
	}

	model.sequence++

	for _, prior := range priors {
		prior.pending++
	}

	model.pending[model.sequence] = pendingAction{
		priors: priors, authority: authority,
	}

	return model.sequence, nil
}

/* prior returns this context's record for an action, creating it on first use. */
func (node *modelContext[Action]) prior(action Action, memory float64) *Prior {
	if node.priors == nil {
		node.priors = make(map[Action]*Prior)
	}

	prior := node.priors[action]

	if prior == nil {
		prior = NewPrior(memory)
		node.priors[action] = prior
	}

	return prior
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

	model.epoch++

	for _, prior := range pending.priors {
		if err := prior.Observe(outcome, pending.authority, model.epoch); err != nil {
			return PriorReading{}, err
		}

		prior.pending--
	}

	delete(model.pending, identity)

	// The reading reported back is the longest context's, which is the one the
	// caller asked about. Shorter prefixes were trained too, and Recall uses
	// them while this one is still too sparse to say anything.
	reading := pending.priors[len(pending.priors)-1].Reading(model.epoch)
	reading.Depth = len(pending.priors) - 1

	return reading, nil
}

/*
Recall prefers the deepest variance-defined reading whose retained input

	authority is at least that of the selected shallower reading. Uniform aging
	leaves Kish support unchanged, so retained input authority is compared separately
	from dispersion and reward signal power. Fresh measured zero outcomes remain evidence.

Lookup first tries the token at the current depth, then scans unused supplied

	tokens in input order for an existing child. This is greedy permutation/subset
	recovery over learned ordered paths, intended to tolerate region-rank jitter;
	it is not strict prefix matching or an exhaustive search of permutations.
	Depth counts matched tokens, not an ordered prefix of the supplied context.
	Lookup creates no evidence.
*/
func (model *Model[Key, Action]) Recall(key Key, context []uint64, action Action) PriorReading {
	node := model.contexts[key]

	if node == nil {
		return PriorReading{}
	}

	reading := PriorReading{}

	if prior := node.priors[action]; prior != nil {
		reading = prior.Reading(model.epoch)
	}

	used := make([]bool, len(context))
	depth := 0

	for depth < len(context) {
		if node.children == nil {
			break
		}

		token := context[depth]
		next := node.children[token]
		matchedIndex := depth

		if next == nil || used[depth] {
			next = nil

			for candidateIndex, candidateToken := range context {
				if used[candidateIndex] {
					continue
				}

				if child := node.children[candidateToken]; child != nil {
					next = child
					matchedIndex = candidateIndex
					break
				}
			}
		}

		if next == nil {
			break
		}

		used[matchedIndex] = true

		node = next
		depth++

		prior := node.priors[action]

		if prior == nil {
			continue
		}

		// Specificity wins ties in retained input authority.
		deeper := prior.Reading(model.epoch)

		if !deeper.Defined {
			continue
		}

		if (deeper.VarianceDefined || !reading.VarianceDefined) &&
			deeper.EvidenceAuthority >= reading.EvidenceAuthority {
			deeper.Depth = depth
			reading = deeper
		}
	}

	return reading
}

/*
Observe incorporates an action outcome directly into all context prefix priors
without requiring an inflight pending ticket. This enables historical warmup
across process restarts while preserving prefix-tree evidence structure.
*/
func (model *Model[Key, Action]) Observe(
	key Key, context []uint64, action Action, outcome, authority float64,
) error {
	if authority <= 0 || authority > 1 {
		return errnie.Err(errnie.Validation, "model: authority must be in (0, 1]", nil)
	}

	node := model.contexts[key]

	if node == nil {
		node = &modelContext[Action]{}
		model.contexts[key] = node
	}

	model.epoch++
	priors := make([]*Prior, 0, len(context)+1)
	priors = append(priors, node.prior(action, model.memory))

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
		priors = append(priors, node.prior(action, model.memory))
	}

	for _, prior := range priors {
		if err := prior.Observe(outcome, authority, model.epoch); err != nil {
			return err
		}
	}

	return nil
}
