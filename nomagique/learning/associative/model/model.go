package model

import (
	"slices"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/learning/associative/prior"
)

/*
Model keeps priors for keyed, ordered contexts and comparable actions. Context
tokens are stable numerical identities supplied by the caller, in order. They
may identify regions or previous-action context; the model interprets neither.
Action includes its parameters in its comparable identity. Retention counts
resolutions within each key, including other actions and contexts in that key.
Unrelated keys cannot erase one another's evidence. A shared related key still
ages on every observation that trains it.

The context trie interns each distinct prefix once. Repeated decisions reuse
its prior records, and pending decisions retain a node reference rather than
mutable grid coordinates or a copied input vector. One owner serializes access.
*/
type Model[Key comparable, Action comparable] struct {
	Contexts map[Key]*modelContext[Action]
	pending  map[uint64]pendingAction
	Sequence uint64
	Memory   float64
	Ordered  bool
}

/* modelContext owns the actions and continuations of one context prefix. */
type modelContext[Action comparable] struct {
	Epoch    uint64
	Children map[uint64]*modelContext[Action]
	Priors   map[Action]*modelPrior
}

/*
pendingAction holds only the evidence fixed when an action was issued. It
retains one record per context depth, because a decision is evidence about
every prefix of the context it was taken under, not only the longest one.
*/
type pendingAction struct {
	priors    []scopedPrior
	authority float64
	depth     int
}

/* scopedPrior binds a prefix's evidence to its root key's resolution clock. */
type scopedPrior struct {
	*modelPrior
	epoch *uint64
}

/* New constructs a keyed prior model with an optional exponential memory window. */
func New[Key comparable, Action comparable](memory ...float64) *Model[Key, Action] {
	model := &Model[Key, Action]{
		Contexts: make(map[Key]*modelContext[Action]),
		pending:  make(map[uint64]pendingAction),
	}

	if len(memory) > 0 && memory[0] > 1 {
		model.Memory = memory[0]
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
context of several jittering identities almost never repeats, so no record ever
reaches a second observation and the model can never leave exploration. Every
prefix carries the same evidence at a coarser resolution, and Recall reads the
deepest usable reading with competitive retained authority, so precision
yields when broader evidence is stronger or fresher.
*/
func (model *Model[Key, Action]) Issue(
	key Key, context []uint64, action Action, authority float64, related ...Key,
) (uint64, error) {
	if !(authority >= 0 && authority <= 1) {
		return 0, errnie.Err(errnie.Validation, "model: authority must be in [0, 1]", nil)
	}

	priors := make([]scopedPrior, 0, (len(context)+1)*(len(related)+1))
	for index, scope := range related {
		if scope == key || slices.Contains(related[:index], scope) {
			return 0, errnie.Err(errnie.Validation, "model: related scope must differ from primary scope", nil)
		}
	}
	for _, scope := range related {
		priors = model.bind(scope, context, action, priors)
	}
	priors = model.bind(key, context, action, priors)

	model.Sequence++

	for _, record := range priors {
		record.pending++
	}

	model.pending[model.Sequence] = pendingAction{
		priors: priors, authority: authority, depth: len(context),
	}

	return model.Sequence, nil
}

/* bind interns one scope's ordered prefixes into the supplied evidence path. */
func (model *Model[Key, Action]) bind(key Key, context []uint64, action Action, priors []scopedPrior) []scopedPrior {
	node := model.Contexts[key]

	if node == nil {
		node = &modelContext[Action]{}
		model.Contexts[key] = node
	}

	epoch := &node.Epoch
	priors = append(priors, scopedPrior{node.record(action, model.Memory), epoch})

	for _, token := range context {
		if node.Children == nil {
			node.Children = make(map[uint64]*modelContext[Action])
		}

		next := node.Children[token]

		if next == nil {
			next = &modelContext[Action]{}
			node.Children[token] = next
		}

		node = next
		priors = append(priors, scopedPrior{node.record(action, model.Memory), epoch})
	}

	return priors
}

/* record returns this context's record for an action, creating it on first use. */
func (node *modelContext[Action]) record(action Action, memory float64) *modelPrior {
	if node.Priors == nil {
		node.Priors = make(map[Action]*modelPrior)
	}

	record := node.Priors[action]

	if record == nil {
		record = &modelPrior{Memory: memory}
		node.Priors[action] = record
	}

	return record
}

/*
Resolve incorporates an issued action's outcome exactly once and releases its
pending record. The caller supplies the numerical target assigned to this
decision, for example its subsequent reward or return-to-go. Shared sequence
returns are correlated targets, not independent evidence of each action's
causal effect. This method estimates assigned targets, not causality.
*/
func (model *Model[Key, Action]) Resolve(identity uint64, outcome float64) (prior.Reading, error) {
	pending, exists := model.pending[identity]

	if !exists {
		return prior.Reading{}, errnie.Err(
			errnie.Validation, "model: action was not issued or is already resolved", nil,
		)
	}

	for index, record := range pending.priors {
		// Each distinct scope occurs once and its prefixes are contiguous.
		if index == 0 || record.epoch != pending.priors[index-1].epoch {
			*record.epoch++
		}

		if err := record.State.Observe(outcome, pending.authority, record.Memory, *record.epoch); err != nil {
			return prior.Reading{}, err
		}

		record.pending--
	}

	delete(model.pending, identity)

	// The reading reported back is the longest context's, which is the one the
	// caller asked about. Shorter prefixes were trained too, and Recall uses
	// them while this one is still too sparse to say anything.
	last := pending.priors[len(pending.priors)-1]
	reading := last.reading(*last.epoch)
	reading.Depth = pending.depth
	reading.ContextLength = pending.depth

	return reading, nil
}

/* Abort releases an unrealized action without creating samples or advancing evidence age. */
func (model *Model[Key, Action]) Abort(identity uint64) error {
	pending, exists := model.pending[identity]

	if !exists {
		return errnie.Err(errnie.Validation, "model: action was not issued or is already finished", nil)
	}

	for _, record := range pending.priors {
		record.pending--
	}

	delete(model.pending, identity)
	return nil
}

/*
Recall prefers the deepest variance-defined reading whose retained input

	authority is at least that of the selected shallower reading. Uniform aging
	leaves Kish support unchanged, so retained input authority is compared separately
	from dispersion and reward signal power. Fresh measured zero outcomes remain evidence.

With Ordered enabled, lookup follows the supplied sequence strictly. Otherwise
lookup first tries the token at the current depth, then scans unused supplied

	tokens in input order for an existing child. This is greedy permutation/subset
	recovery over learned ordered paths, intended to tolerate region-rank jitter;
	it is not strict prefix matching or an exhaustive search of permutations.
	Depth counts matched tokens, not an ordered prefix of the supplied context.
	Lookup creates no evidence.
*/
func (model *Model[Key, Action]) Recall(key Key, context []uint64, action Action) prior.Reading {
	node := model.Contexts[key]

	if node == nil {
		return prior.Reading{ContextLength: len(context)}
	}

	epoch := node.Epoch
	reading := prior.Reading{}

	if record := node.Priors[action]; record != nil {
		reading = record.reading(epoch)
	}
	reading.ContextLength = len(context)

	used := make([]bool, len(context))
	depth := 0

	for depth < len(context) {
		if node.Children == nil {
			break
		}

		token := context[depth]
		next := node.Children[token]
		matchedIndex := depth

		if model.Ordered && next == nil {
			break
		}

		if next == nil || used[depth] {
			next = nil

			for candidateIndex, candidateToken := range context {
				if used[candidateIndex] {
					continue
				}

				if child := node.Children[candidateToken]; child != nil {
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

		record := node.Priors[action]

		if record == nil {
			continue
		}

		// Specificity wins ties in retained input authority.
		deeper := record.reading(epoch)

		if !deeper.Defined {
			continue
		}

		if reading.Defined && !reading.Provisional && deeper.Provisional {
			continue
		}

		if (reading.Provisional && !deeper.Provisional) ||
			((deeper.VarianceDefined || !reading.VarianceDefined) &&
				deeper.EvidenceAuthority >= reading.EvidenceAuthority) {
			deeper.Depth = depth
			deeper.ContextLength = len(context)
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
	key Key, context []uint64, action Action, outcome, authority float64, related ...Key,
) error {
	if !(authority >= 0 && authority <= 1) {
		return errnie.Err(errnie.Validation, "model: authority must be in [0, 1]", nil)
	}

	priors := make([]scopedPrior, 0, (len(context)+1)*(len(related)+1))
	for index, scope := range related {
		if scope == key || slices.Contains(related[:index], scope) {
			return errnie.Err(errnie.Validation, "model: related scope must differ from primary scope", nil)
		}
	}
	for _, scope := range related {
		priors = model.bind(scope, context, action, priors)
	}
	priors = model.bind(key, context, action, priors)
	for index, record := range priors {
		if index == 0 || record.epoch != priors[index-1].epoch {
			*record.epoch++
		}

		if err := record.State.Observe(outcome, authority, record.Memory, *record.epoch); err != nil {
			return err
		}
	}

	return nil
}
